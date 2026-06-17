#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from datetime import date
from pathlib import Path

from rencrow_data import db
from rencrow_data.backtest import BacktestOptions, run_weekly_rotation_backtest
from rencrow_data.quality import parse_csv_values


def _parse_date(value: str | None) -> date | None:
    if not value:
        return None
    return date.fromisoformat(value)


def _resolve_snapshot(con, value: str) -> str:
    if value == "latest":
        row = con.execute(
            "SELECT snapshot_id FROM snapshot_registry ORDER BY snapshot_date DESC, snapshot_id DESC LIMIT 1"
        ).fetchone()
        if row is None:
            raise ValueError("cannot resolve --snapshot latest because snapshot_registry is empty")
        return str(row["snapshot_id"])
    row = con.execute("SELECT snapshot_id FROM snapshot_registry WHERE snapshot_id=?", (value,)).fetchone()
    if row is None:
        raise ValueError(f"snapshot not found: {value}")
    return value


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db_path", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--strategy", default="weekly_etf_rotation_v1")
    parser.add_argument("--start")
    parser.add_argument("--end")
    parser.add_argument("--cost-bps", type=float, default=10.0)
    parser.add_argument("--slippage-bps", type=float, default=0.0)
    parser.add_argument("--tax-mode", choices=("none", "approx_jp_taxable"), default="none")
    parser.add_argument("--walk-forward", action="store_true")
    parser.add_argument("--output-dir", default="rencrow-data/data/backtests")
    parser.add_argument("--symbols", action="append", help="Comma-separated symbols overriding the strategy universe.")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    con = db.connect(args.db_path)
    db.init_schema(con)
    try:
        snapshot_id = _resolve_snapshot(con, args.snapshot)
        options = BacktestOptions(
            snapshot_id=snapshot_id,
            strategy_id=args.strategy,
            start=_parse_date(args.start),
            end=_parse_date(args.end),
            cost_bps=args.cost_bps,
            slippage_bps=args.slippage_bps,
            tax_mode=args.tax_mode,
            mode="walk_forward" if args.walk_forward else "full",
            output_dir=Path(args.output_dir),
            symbols=parse_csv_values(args.symbols),
        )
        result = run_weekly_rotation_backtest(con, options)
    except KeyError as exc:
        print(f"config error: {exc}", file=sys.stderr)
        raise SystemExit(4)
    except ValueError as exc:
        print(f"data error: {exc}", file=sys.stderr)
        raise SystemExit(3)
    finally:
        con.close()

    if args.json:
        print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    else:
        metrics = result["metrics"]
        print(
            "backtest success "
            f"id={result['backtest_id']} strategy={result['strategy_id']} snapshot={result['snapshot_id']} "
            f"weeks={result['weeks']} final_equity={metrics['final_equity']:.6f} "
            f"cagr={metrics['cagr']:.6f} max_dd={metrics['max_dd']:.6f}"
        )


if __name__ == "__main__":
    main()
