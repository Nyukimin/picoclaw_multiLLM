#!/usr/bin/env python3
from __future__ import annotations

import argparse

from rencrow_data import db
from rencrow_data.config import config_path, load_config
from rencrow_data.macro import ingest_calendar_csv, ingest_macro_csv, ingest_macro_source


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--config-root", default="rencrow-data/config")
    parser.add_argument("--data-root", default="rencrow-data")
    parser.add_argument("--mode", choices=("fixture", "online", "backfill", "hybrid", "incremental"), default="fixture")
    parser.add_argument("--start-date")
    parser.add_argument("--end-date")
    parser.add_argument("--lookback-days", type=int, default=30)
    args = parser.parse_args()

    con = db.connect(args.db)
    sources = load_config(config_path(args.config_root, "sources.yml"), default={"macro_sources": []})
    calendars = load_config(config_path(args.config_root, "calendars.yml"), default={"calendar_sources": []})
    failures = 0
    total_sources = 0
    for source in sources.get("macro_sources", []):
        total_sources += 1
        if args.mode == "fixture" or not source.get("provider"):
            rows, status = ingest_macro_csv(con, source, args.data_root)
        else:
            rows, status = ingest_macro_source(
                con,
                source,
                args.data_root,
                mode=args.mode,
                start_date=args.start_date,
                end_date=args.end_date,
                lookback_days=args.lookback_days,
            )
        failures += 1 if status != "success" else 0
        print(f"macro {source.get('source_name')}: {status} rows={rows}")
    for source in calendars.get("calendar_sources", []):
        total_sources += 1
        rows, status = ingest_calendar_csv(con, source, args.data_root)
        failures += 1 if status != "success" else 0
        print(f"calendar {source.get('source_name')}: {status} rows={rows}")
    if failures and total_sources == 0:
        raise SystemExit(1)
    if failures:
        print(f"macro ingest complete partial_failures={failures}")
    con.close()


if __name__ == "__main__":
    main()
