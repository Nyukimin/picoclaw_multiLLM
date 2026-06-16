#!/usr/bin/env python3
from __future__ import annotations

import argparse
from datetime import datetime

from rencrow_data import db
from rencrow_data.snapshot import make_snapshot


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--output-dir", default="rencrow-data/data/snapshots")
    parser.add_argument("--snapshot-date", default=datetime.utcnow().date().isoformat())
    args = parser.parse_args()
    con = db.connect(args.db)
    result = make_snapshot(con, args.db, args.output_dir, args.snapshot_date)
    print(f"snapshot {result['status']} path={result['path']} db_hash={result['db_hash']}")
    con.close()


if __name__ == "__main__":
    main()

