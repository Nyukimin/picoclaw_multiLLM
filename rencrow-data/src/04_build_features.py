#!/usr/bin/env python3
from __future__ import annotations

import argparse

from rencrow_data import db
from rencrow_data.features import build_features


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--week-end", help="Accepted for CLI compatibility; current builder rebuilds all weekly features.")
    args = parser.parse_args()
    con = db.connect(args.db)
    db.init_schema(con)
    count = build_features(con)
    print(f"feature build complete rows={count}")
    con.close()


if __name__ == "__main__":
    main()
