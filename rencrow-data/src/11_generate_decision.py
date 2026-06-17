#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from rencrow_data import db
from rencrow_data.decision import DecisionOptions, generate_decision


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


def _resolve_risk_check(con, value: str, snapshot_id: str, strategy_id: str) -> str:
    if value == "latest":
        row = con.execute(
            """
            SELECT risk_check_id
              FROM risk_check_result
             WHERE snapshot_id=? AND strategy_id=?
             ORDER BY created_at DESC
             LIMIT 1
            """,
            (snapshot_id, strategy_id),
        ).fetchone()
        if row is None:
            raise ValueError(f"cannot resolve --risk-check latest for snapshot={snapshot_id} strategy={strategy_id}")
        return row["risk_check_id"]
    row = con.execute(
        "SELECT risk_check_id FROM risk_check_result WHERE risk_check_id=? AND snapshot_id=? AND strategy_id=?",
        (value, snapshot_id, strategy_id),
    ).fetchone()
    if row is None:
        raise ValueError(f"risk check not found or mismatched: {value}")
    return value


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db_path", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--strategy", default="weekly_etf_rotation_v1")
    parser.add_argument("--risk-check", default="latest")
    parser.add_argument("--output-dir", default="rencrow-data/approvals")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    con = db.connect(args.db_path)
    db.init_schema(con)
    try:
        snapshot_id = _resolve_snapshot(con, args.snapshot)
        risk_check_id = _resolve_risk_check(con, args.risk_check, snapshot_id, args.strategy)
        result = generate_decision(
            con,
            DecisionOptions(
                snapshot_id=snapshot_id,
                strategy_id=args.strategy,
                risk_check_id=risk_check_id,
                output_dir=Path(args.output_dir),
            ),
        )
    except ValueError as exc:
        print(f"data error: {exc}", file=sys.stderr)
        raise SystemExit(4)
    finally:
        con.close()

    if args.json:
        print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    else:
        print(
            "decision candidate "
            f"id={result['decision_id']} strategy={result['strategy_id']} snapshot={result['snapshot_id']} "
            f"risk={result['risk_status']} vetoed={result['vetoed']} approval={result['approval_path']}"
        )


if __name__ == "__main__":
    main()
