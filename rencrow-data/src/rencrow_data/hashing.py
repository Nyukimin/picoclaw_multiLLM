from __future__ import annotations

import hashlib
import sqlite3
from pathlib import Path


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: str | Path) -> str:
    h = hashlib.sha256()
    with Path(path).open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def stable_table_hash(con: sqlite3.Connection, table: str, order_by: str) -> str:
    rows = con.execute(f"SELECT * FROM {table} ORDER BY {order_by}").fetchall()
    h = hashlib.sha256()
    for row in rows:
        h.update("|".join("" if v is None else str(v) for v in tuple(row)).encode("utf-8"))
        h.update(b"\n")
    return h.hexdigest()


def stable_db_hash(con: sqlite3.Connection) -> str:
    tables = [
        ("instruments", "instrument_id"),
        ("source_fetch_log", "fetch_id"),
        ("price_raw", "instrument_id, trade_date, source_name"),
        ("corporate_action", "instrument_id, action_date, action_type, source_name"),
        ("macro_series", "series_code, obs_date, vintage_date, source_name"),
        ("economic_calendar", "event_date, category, event_name"),
        ("etf_holding_snapshot", "instrument_id, snapshot_date, constituent_code, source_name"),
        ("feature_weekly", "instrument_id, week_end"),
        ("event_log", "event_id"),
    ]
    h = hashlib.sha256()
    for table, order_by in tables:
        h.update(table.encode("utf-8"))
        h.update(stable_table_hash(con, table, order_by).encode("ascii"))
    return h.hexdigest()

