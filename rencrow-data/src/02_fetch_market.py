#!/usr/bin/env python3
from __future__ import annotations

import argparse

from rencrow_data import db
from rencrow_data.config import config_path, load_config
from rencrow_data.market import save_market_csv, save_market_item

ALLOWED_ASSET_TYPES = ("ETF", "STOCK", "CASH_PROXY", "CRYPTO", "INDEX")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--config-root", default="rencrow-data/config")
    parser.add_argument("--data-root", default="rencrow-data")
    parser.add_argument("--mode", choices=("fixture", "online", "backfill", "hybrid", "incremental"), default="fixture")
    parser.add_argument("--start-date")
    parser.add_argument("--end-date")
    parser.add_argument("--lookback-days", type=int, default=7)
    args = parser.parse_args()

    con = db.connect(args.db)
    db.init_schema(con)
    config = load_config(config_path(args.config_root, "instruments.yml"), default={"instruments": []})
    total = 0
    failures = 0
    for item in config.get("instruments", []):
        if item.get("asset_type") not in ALLOWED_ASSET_TYPES:
            continue
        if args.mode == "fixture":
            rows, status = save_market_csv(con, item, args.data_root)
        else:
            rows, status = save_market_item(
                con,
                item,
                args.data_root,
                mode=args.mode,
                start_date=args.start_date,
                end_date=args.end_date,
                lookback_days=args.lookback_days,
            )
        total += rows
        failures += 1 if status != "success" else 0
        print(f"{item['symbol']}: {status} rows={rows}")
    if failures and total == 0:
        raise SystemExit(1)
    if failures:
        print(f"market ingest complete rows={total} partial_failures={failures}")
    else:
        print(f"market ingest complete rows={total}")
    con.close()


if __name__ == "__main__":
    main()
