from __future__ import annotations

import json
import math
from datetime import timedelta

from . import db
from .timeutil import parse_date, utcnow_iso


def detect_events(con, stale_hours: int = 48) -> int:
    count = 0
    bad_fetches = con.execute(
        """
        SELECT fetch_id, source_name, status, error_message
        FROM source_fetch_log
        WHERE status IN ('fail', 'partial')
        """
    ).fetchall()
    for row in bad_fetches:
        db.log_event(
            con,
            "system",
            "stop",
            "source_fetch_unresolved",
            event_risk_score=1.0,
            context_json=json.dumps(dict(row), ensure_ascii=False),
        )
        count += 1

    weeks = con.execute("SELECT DISTINCT week_end FROM feature_weekly ORDER BY week_end").fetchall()
    for wrow in weeks:
        week_end = parse_date(wrow["week_end"])
        start = (week_end - timedelta(days=2)).isoformat()
        end = (week_end + timedelta(days=2)).isoformat()
        events = con.execute(
            "SELECT category, event_name, importance FROM economic_calendar WHERE event_date BETWEEN ? AND ?",
            (start, end),
        ).fetchall()
        max_score = 0.0
        for ev in events:
            score = 0.7 if ev["importance"] in ("high", "critical") else 0.4
            level = "warn" if score < 0.9 else "stop"
            db.log_event(
                con,
                "macro",
                level,
                f"calendar_{ev['category'].lower()}",
                event_risk_score=score,
                context_json=json.dumps({"week_end": week_end.isoformat(), "event": dict(ev)}, ensure_ascii=False),
            )
            max_score = max(max_score, score)
            count += 1
        if max_score:
            con.execute(
                "UPDATE feature_weekly SET event_risk_score=max(COALESCE(event_risk_score, 0), ?) WHERE week_end=?",
                (max_score, week_end.isoformat()),
            )

    price_rows = con.execute(
        """
        SELECT instrument_id, trade_date, adj_close, volume
        FROM price_raw
        ORDER BY instrument_id, trade_date
        """
    ).fetchall()
    by_inst = {}
    for row in price_rows:
        by_inst.setdefault(int(row["instrument_id"]), []).append(row)
    for iid, rows in by_inst.items():
        rets = []
        vols = []
        for idx in range(1, len(rows)):
            prev = rows[idx - 1]["adj_close"]
            curr = rows[idx]["adj_close"]
            if prev in (None, 0) or curr is None:
                continue
            ret = float(curr) / float(prev) - 1.0
            rets.append(ret)
            vols.append(float(rows[idx]["volume"] or 0.0))
            if len(rets) >= 20:
                window = rets[-20:]
                mean = sum(window) / len(window)
                var = sum((x - mean) ** 2 for x in window) / max(1, len(window) - 1)
                sd = math.sqrt(var)
                z = 0.0 if sd == 0 else (ret - mean) / sd
                if abs(z) >= 2.5 or abs(ret) >= 0.05:
                    score = 1.0 if abs(ret) >= 0.05 else 0.7
                    db.log_event(
                        con,
                        "market",
                        "warn",
                        "return_spike",
                        value=ret,
                        event_risk_score=score,
                        context_json=json.dumps({"instrument_id": iid, "date": rows[idx]["trade_date"], "z": z}, ensure_ascii=False),
                    )
                count += 1
        if len(vols) >= 20 and vols[-1] > 2.0 * (sum(vols[-20:]) / 20):
            db.log_event(
                con,
                "market",
                "warn",
                "volume_spike",
                value=vols[-1],
                event_risk_score=0.5,
                context_json=json.dumps({"instrument_id": iid, "event_ts": utcnow_iso()}, ensure_ascii=False),
            )
            count += 1
    con.commit()
    return count
