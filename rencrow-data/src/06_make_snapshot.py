#!/usr/bin/env python3
from __future__ import annotations

import argparse
from datetime import datetime

from rencrow_data import db
from rencrow_data.snapshot import make_snapshot


def _resolve_snapshot_date(con, snapshot_date: str | None, week_end: str | None) -> str:
    value = week_end or snapshot_date
    if value in (None, "today"):
        return datetime.utcnow().date().isoformat()
    if value == "latest":
        row = con.execute("SELECT MAX(week_end) AS week_end FROM feature_weekly").fetchone()
        if row is None or row["week_end"] is None:
            return datetime.utcnow().date().isoformat()
        return row["week_end"]
    return value


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--output-dir", default="rencrow-data/data/snapshots")
    parser.add_argument("--snapshot-date")
    parser.add_argument("--week-end")
    args = parser.parse_args()
    con = db.connect(args.db)
    db.init_schema(con)
    snapshot_date = _resolve_snapshot_date(con, args.snapshot_date, args.week_end)
    result = make_snapshot(con, args.db, args.output_dir, snapshot_date)
    print(f"snapshot {result['status']} path={result['path']} db_hash={result['db_hash']}")
    con.close()


if __name__ == "__main__":
    main()
