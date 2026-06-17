from __future__ import annotations

import json
from pathlib import Path
import math
from datetime import timedelta

from . import db
from .timeutil import parse_date, utcnow_iso


def _latest_fetch_statuses(con):
    return con.execute(
        """
        SELECT f.source_name, f.status, f.error_message, f.fetch_id
        FROM source_fetch_log f
        JOIN (
          SELECT source_name, MAX(fetch_id) AS fetch_id
          FROM source_fetch_log
          GROUP BY source_name
        ) latest
          ON latest.source_name=f.source_name AND latest.fetch_id=f.fetch_id
        """
    ).fetchall()


def _active_source_names() -> set[str]:
    root = Path(__file__).resolve().parents[2]
    names: set[str] = set()
    for rel in ("config/instruments.yml", "config/sources.yml", "config/calendars.yml"):
        path = root / rel
        if not path.exists():
            continue
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            continue
        if isinstance(data, dict):
            for key in ("instruments", "macro_sources", "calendar_sources"):
                items = data.get(key) or []
                for item in items:
                    if isinstance(item, dict):
                        source_name = item.get("source_name")
                        if source_name:
                            names.add(str(source_name))
    return names


def detect_events(con, stale_hours: int = 48) -> int:
    count = 0
    active_sources = _active_source_names()
    latest_fetches = _latest_fetch_statuses(con)
    current_bad = []
    for row in latest_fetches:
        if row["source_name"] in active_sources and row["status"] in ("fail", "partial"):
            current_bad.append(row)
    open_fetch_events = con.execute(
        "SELECT event_id, context_json FROM event_log WHERE level='stop' AND reason='source_fetch_unresolved' AND resolved_at IS NULL"
    ).fetchall()
    open_by_source = {}
    for row in open_fetch_events:
        try:
            ctx = json.loads(row["context_json"] or "{}")
        except Exception:
            continue
        source_name = ctx.get("source_name")
        if source_name:
            open_by_source.setdefault(source_name, []).append(int(row["event_id"]))
    current_bad_sources = {row["source_name"] for row in current_bad}
    for source_name, event_ids in open_by_source.items():
        if source_name not in current_bad_sources:
            con.execute(
                "UPDATE event_log SET resolved_at=? WHERE event_id IN (%s)" % ",".join("?" for _ in event_ids),
                [utcnow_iso(), *event_ids],
            )
            count += len(event_ids)
    for row in current_bad:
        if row["source_name"] in open_by_source:
            continue
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
            if ev["importance"] == "critical":
                score = 1.0
            elif ev["importance"] == "high":
                score = 0.7
            else:
                score = 0.4
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
