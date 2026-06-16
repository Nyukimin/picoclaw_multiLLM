#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from rencrow_data import db
from rencrow_data.config import config_path, load_config


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--config-root", default="rencrow-data/config")
    args = parser.parse_args()

    con = db.connect(args.db)
    db.init_schema(con)
    instruments = load_config(config_path(args.config_root, "instruments.yml"), default={"instruments": []})
    count = db.upsert_instruments(con, instruments.get("instruments", []))
    db.log_event(con, "system", "info", "schema_initialized", value=count, event_risk_score=0.0)
    print(f"initialized db={Path(args.db)} instruments={count}")
    con.close()


if __name__ == "__main__":
    main()

