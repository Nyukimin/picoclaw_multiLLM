#!/usr/bin/env python3
from __future__ import annotations

import argparse

from rencrow_data import db
from rencrow_data.events import detect_events


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", default="rencrow-data/data/rencrow.db")
    args = parser.parse_args()
    con = db.connect(args.db)
    count = detect_events(con)
    print(f"event detection complete rows={count}")
    con.close()


if __name__ == "__main__":
    main()

