#!/usr/bin/env python3
from __future__ import annotations

import argparse

from rencrow_data import db
from rencrow_data.events import detect_events


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--week-end", help="Accepted for CLI compatibility; current detector scans available weekly features.")
    args = parser.parse_args()
    con = db.connect(args.db)
    db.init_schema(con)
    count = detect_events(con)
    print(f"event detection complete rows={count}")
    con.close()


if __name__ == "__main__":
    main()
