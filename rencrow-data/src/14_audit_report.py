#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from rencrow_data import db
from rencrow_data.audit import AuditOptions, build_audit_report


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


def _parse_decision(value: str | None) -> int | None:
    if not value or value == "latest":
        return None
    return int(value)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", "--db-path", dest="db_path", default="rencrow-data/data/rencrow.db")
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--decision")
    parser.add_argument("--paper-latest", action="store_true")
    parser.add_argument("--output-dir", default="rencrow-data/reports")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    con = db.connect(args.db_path)
    db.init_schema(con)
    try:
        snapshot_id = _resolve_snapshot(con, args.snapshot)
        result = build_audit_report(
            con,
            AuditOptions(
                snapshot_id=snapshot_id,
                decision_id=_parse_decision(args.decision),
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
            "audit report "
            f"snapshot={result['snapshot_id']} decision={result['decision_id']} "
            f"risk={result['risk_status']} output={result['output_path']}"
        )


if __name__ == "__main__":
    main()
