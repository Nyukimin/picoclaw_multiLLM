from __future__ import annotations

import gzip
import json
import shutil
import sqlite3
import tempfile
from pathlib import Path

from .hashing import stable_db_hash, stable_table_hash
from .timeutil import utcnow_iso


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
                for item in data.get(key) or []:
                    if isinstance(item, dict) and item.get("source_name"):
                        names.add(str(item["source_name"]))
    return names


def _event_state(con: sqlite3.Connection) -> dict:
    rows = con.execute(
        "SELECT level, reason, COUNT(*) AS n FROM event_log WHERE resolved_at IS NULL GROUP BY level, reason ORDER BY level, reason"
    ).fetchall()
    return {"open_events": [dict(r) for r in rows]}


def precheck_status(con: sqlite3.Connection) -> tuple[str, str]:
    active_sources = _active_source_names()
    latest_fetches = con.execute(
        """
        SELECT f.source_name, f.status, f.fetch_id
        FROM source_fetch_log f
        JOIN (
          SELECT source_name, MAX(fetch_id) AS fetch_id
          FROM source_fetch_log
          GROUP BY source_name
        ) latest
          ON latest.source_name=f.source_name AND latest.fetch_id=f.fetch_id
        """
    ).fetchall()
    bad_fetch = sum(1 for row in latest_fetches if row["source_name"] in active_sources and row["status"] in ("fail", "partial"))
    stop_events = 0
    if active_sources:
        for row in con.execute(
            "SELECT context_json FROM event_log WHERE level='stop' AND resolved_at IS NULL AND reason='source_fetch_unresolved'"
        ).fetchall():
            try:
                ctx = json.loads(row["context_json"] or "{}")
            except Exception:
                continue
            if ctx.get("source_name") in active_sources:
                stop_events += 1
    high_risk = con.execute("SELECT COUNT(*) FROM feature_weekly WHERE COALESCE(event_risk_score, 0) >= 0.9").fetchone()[0]
    if bad_fetch or stop_events or high_risk:
        return "blocked", f"bad_fetch={bad_fetch}; stop_events={stop_events}; high_risk_features={high_risk}"
    return "success", "precheck passed"


def make_snapshot(con: sqlite3.Connection, db_path: str | Path, output_dir: str | Path, snapshot_date: str) -> dict:
    out_dir = Path(output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    snapshot_path = out_dir / f"snapshot_{snapshot_date.replace('-', '')}.sqlite.gz"
    status, notes = precheck_status(con)
    db_hash = stable_db_hash(con)
    features_hash = stable_table_hash(con, "feature_weekly", "instrument_id, week_end")
    data_range = con.execute("SELECT MIN(trade_date), MAX(trade_date) FROM price_raw").fetchone()
    source_rows = con.execute("SELECT source_name, status, COUNT(*) AS n FROM source_fetch_log GROUP BY source_name, status").fetchall()
    missing_rate = 0.0
    event_state = _event_state(con)

    with tempfile.NamedTemporaryFile(suffix=".sqlite", delete=False) as tmp:
        tmp_path = Path(tmp.name)
    try:
        src = sqlite3.connect(db_path)
        dst = sqlite3.connect(tmp_path)
        src.backup(dst)
        dst.close()
        src.close()
        with tmp_path.open("rb") as f_in, gzip.open(snapshot_path, "wb") as f_out:
            shutil.copyfileobj(f_in, f_out)
    finally:
        tmp_path.unlink(missing_ok=True)

    con.execute(
        """
        INSERT INTO snapshot_registry(
          snapshot_date, snapshot_path, db_hash, features_hash, source_summary_json,
          data_start_date, data_end_date, missing_rate, event_state_json, status, notes, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(snapshot_date) DO UPDATE SET
          snapshot_path=excluded.snapshot_path,
          db_hash=excluded.db_hash,
          features_hash=excluded.features_hash,
          source_summary_json=excluded.source_summary_json,
          data_start_date=excluded.data_start_date,
          data_end_date=excluded.data_end_date,
          missing_rate=excluded.missing_rate,
          event_state_json=excluded.event_state_json,
          status=excluded.status,
          notes=excluded.notes,
          created_at=excluded.created_at
        """,
        (
            snapshot_date,
            str(snapshot_path),
            db_hash,
            features_hash,
            json.dumps([dict(r) for r in source_rows], ensure_ascii=False),
            data_range[0],
            data_range[1],
            missing_rate,
            json.dumps(event_state, ensure_ascii=False),
            status,
            notes,
            utcnow_iso(),
        ),
    )
    con.commit()
    return {"path": str(snapshot_path), "status": status, "db_hash": db_hash, "features_hash": features_hash, "notes": notes}
