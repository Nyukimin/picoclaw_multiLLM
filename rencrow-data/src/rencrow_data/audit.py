from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class AuditOptions:
    snapshot_id: str
    decision_id: int | None = None
    output_dir: Path | None = None
    paper_latest: bool = False


def _snapshot(con, snapshot_id: str):
    row = con.execute("SELECT * FROM snapshot_registry WHERE snapshot_id=?", (snapshot_id,)).fetchone()
    if row is None:
        raise ValueError(f"snapshot not found: {snapshot_id}")
    return row


def _latest_decision(con, snapshot_id: str) -> int | None:
    row = con.execute(
        "SELECT decision_id FROM decision_log WHERE snapshot_id=? ORDER BY created_at DESC, decision_id DESC LIMIT 1",
        (int(snapshot_id),),
    ).fetchone()
    return None if row is None else int(row["decision_id"])


def _latest_paper_decision(con, snapshot_id: str) -> int | None:
    row = con.execute(
        """
        SELECT d.decision_id
          FROM decision_log d
          JOIN paper_trade_log p ON p.decision_id=d.decision_id
         WHERE d.snapshot_id=?
           AND d.account_scope='paper'
         ORDER BY p.created_at DESC, p.paper_trade_id DESC
         LIMIT 1
        """,
        (int(snapshot_id),),
    ).fetchone()
    return None if row is None else int(row["decision_id"])


def _count(con, sql: str, params: tuple[object, ...] = ()) -> int:
    row = con.execute(sql, params).fetchone()
    return int(row[0] or 0)


def _paper_gate(con) -> dict[str, object]:
    paper_decisions = con.execute(
        """
        SELECT d.decision_id, d.snapshot_id, d.decision_date, s.snapshot_date
          FROM decision_log d
          LEFT JOIN snapshot_registry s ON s.snapshot_id=d.snapshot_id
         WHERE d.account_scope='paper'
         ORDER BY d.decision_date, d.decision_id
        """
    ).fetchall()
    paper_weeks = _count(
        con,
        """
        SELECT COUNT(DISTINCT d.decision_date)
          FROM decision_log d
          JOIN paper_trade_log p ON p.decision_id=d.decision_id
         WHERE d.account_scope='paper'
        """,
    )
    missing_decision_paper = _count(
        con,
        """
        SELECT COUNT(*)
          FROM decision_log d
         WHERE d.account_scope='paper'
           AND NOT EXISTS (SELECT 1 FROM paper_trade_log p WHERE p.decision_id=d.decision_id)
        """,
    )
    llm_audit_rows = _count(con, "SELECT COUNT(*) FROM llm_audit_log")
    no_trade_rows = _count(con, "SELECT COUNT(*) FROM paper_trade_log WHERE status IN ('vetoed', 'no_candidate', 'zero_weight')")
    event_veto_rows = _count(con, "SELECT COUNT(*) FROM weekly_signal WHERE vetoed=1")
    missing_logs = {
        "snapshot": 0,
        "validation": 0,
        "feature": 0,
        "backtest": 0,
        "risk": 0,
        "paper_trade": 0,
        "report": 0,
    }
    weeks: list[dict[str, object]] = []
    for row in paper_decisions:
        decision_id = int(row["decision_id"])
        snapshot_id = row["snapshot_id"]
        snapshot_date = row["snapshot_date"] or row["decision_date"]
        week_status = {
            "decision_id": decision_id,
            "snapshot_id": snapshot_id,
            "snapshot_date": snapshot_date,
            "validation": False,
            "feature": False,
            "backtest": False,
            "risk": False,
            "paper_trade": False,
            "report": False,
            "complete": False,
            "missing": [],
        }
        if snapshot_id is None:
            missing_logs["snapshot"] += 1
            week_status["missing"] = ["snapshot"]
            weeks.append(week_status)
            continue
        validation_count = _count(con, "SELECT COUNT(*) FROM data_quality_check WHERE check_date=?", (snapshot_date,))
        feature_count = _count(con, "SELECT COUNT(*) FROM feature_weekly WHERE week_end<=?", (snapshot_date,))
        backtest_count = _count(con, "SELECT COUNT(*) FROM backtest_run WHERE snapshot_id=?", (str(snapshot_id),))
        risk_count = _count(con, "SELECT COUNT(*) FROM risk_check_result WHERE decision_id=?", (str(decision_id),))
        paper_trade_count = _count(con, "SELECT COUNT(*) FROM paper_trade_log WHERE decision_id=?", (decision_id,))
        report_count = _count(con, "SELECT COUNT(*) FROM llm_audit_log WHERE snapshot_id=?", (str(snapshot_id),))
        week_status.update(
            {
                "validation": validation_count > 0,
                "feature": feature_count > 0,
                "backtest": backtest_count > 0,
                "risk": risk_count > 0,
                "paper_trade": paper_trade_count > 0,
                "report": report_count > 0,
            }
        )
        missing_for_week: list[str] = []
        if validation_count == 0:
            missing_logs["validation"] += 1
            missing_for_week.append("validation")
        if feature_count == 0:
            missing_logs["feature"] += 1
            missing_for_week.append("feature")
        if backtest_count == 0:
            missing_logs["backtest"] += 1
            missing_for_week.append("backtest")
        if risk_count == 0:
            missing_logs["risk"] += 1
            missing_for_week.append("risk")
        if paper_trade_count == 0:
            missing_logs["paper_trade"] += 1
            missing_for_week.append("paper_trade")
        if report_count == 0:
            missing_logs["report"] += 1
            missing_for_week.append("report")
        week_status["missing"] = missing_for_week
        week_status["complete"] = not missing_for_week
        weeks.append(week_status)
    missing_weekly_logs = sum(missing_logs.values())
    if paper_weeks >= 12 and missing_decision_paper == 0 and missing_weekly_logs == 0:
        status = "preferred_ready"
    elif paper_weeks >= 8 and missing_decision_paper == 0 and missing_weekly_logs == 0:
        status = "minimum_ready"
    else:
        status = "not_ready"
    return {
        "status": status,
        "paper_weeks": paper_weeks,
        "missing_decision_paper": missing_decision_paper,
        "llm_audit_rows": llm_audit_rows,
        "no_trade_rows": no_trade_rows,
        "event_veto_rows": event_veto_rows,
        "missing_weekly_logs": missing_weekly_logs,
        "missing_logs": missing_logs,
        "weeks": weeks,
    }


def build_audit_report(con, options: AuditOptions) -> dict[str, object]:
    snapshot = _snapshot(con, options.snapshot_id)
    if options.decision_id is not None:
        decision_id = options.decision_id
    elif options.paper_latest:
        decision_id = _latest_paper_decision(con, options.snapshot_id) or _latest_decision(con, options.snapshot_id)
    else:
        decision_id = _latest_decision(con, options.snapshot_id)
    snapshot_date = snapshot["snapshot_date"]
    fetch_failures = _count(con, "SELECT COUNT(*) FROM source_fetch_log WHERE status='fail'")
    fetch_partials = _count(con, "SELECT COUNT(*) FROM source_fetch_log WHERE status='partial'")
    quality_blockers = _count(
        con,
        "SELECT COUNT(*) FROM data_quality_check WHERE check_date=? AND severity='blocker' AND status!='pass'",
        (snapshot_date,),
    )
    risk = None
    if decision_id is not None:
        risk = con.execute(
            """
            SELECT r.*
              FROM risk_check_result r
              JOIN decision_log d ON CAST(d.decision_id AS TEXT)=r.decision_id
             WHERE d.decision_id=?
             ORDER BY r.created_at DESC
             LIMIT 1
            """,
            (decision_id,),
        ).fetchone()
    decision = None if decision_id is None else con.execute("SELECT * FROM decision_log WHERE decision_id=?", (decision_id,)).fetchone()
    paper_count = 0 if decision_id is None else _count(con, "SELECT COUNT(*) FROM paper_trade_log WHERE decision_id=?", (decision_id,))
    tax_lot_count = 0 if decision_id is None else _count(
        con,
        """
        SELECT COUNT(*)
          FROM tax_lot_log t
          JOIN paper_trade_log p ON p.paper_trade_id=t.source_order_id
         WHERE p.decision_id=?
        """,
        (decision_id,),
    )
    paper_gate = _paper_gate(con)

    lines = [
        "# RenCrow Investment Audit Report",
        "",
        "## Snapshot",
        "",
        f"- snapshot_id: {options.snapshot_id}",
        f"- snapshot_date: {snapshot_date}",
        f"- status: {snapshot['status']}",
        f"- db_hash: {snapshot['db_hash'] or ''}",
        f"- features_hash: {snapshot['features_hash'] or ''}",
        "",
        "## Data Quality",
        "",
        f"- fetch_failures_total: {fetch_failures}",
        f"- fetch_partials_total: {fetch_partials}",
        f"- quality_blockers_on_snapshot_date: {quality_blockers}",
        "",
        "## Risk",
        "",
    ]
    if risk is None:
        lines.append("- risk_check: not_found")
    else:
        detail = json.loads(risk["detail_json"] or "{}")
        lines.extend(
            [
                f"- risk_check_id: {risk['risk_check_id']}",
                f"- status: {risk['status']}",
                f"- max_dd_check: {risk['max_dd_check']}",
                f"- weekly_loss_check: {risk['weekly_loss_check']}",
                f"- volatility_check: {risk['volatility_check']}",
                f"- event_check: {risk['event_check']}",
                f"- quality_blockers: {detail.get('quality_blockers', '')}",
                f"- event_blockers: {detail.get('event_blockers', '')}",
            ]
        )
    lines.extend(["", "## Decision", ""])
    if decision is None:
        lines.append("- decision: not_found")
    else:
        candidate = json.loads(decision["candidate_json"] or "{}")
        veto = json.loads(decision["veto_json"] or "{}")
        lines.extend(
            [
                f"- decision_id: {decision['decision_id']}",
                f"- account_scope: {decision['account_scope']}",
                f"- approved: {decision['approved']}",
                f"- approver: {decision['approver'] or ''}",
                f"- approved_at: {decision['approved_at'] or ''}",
                f"- approval_reason: {decision['approval_reason'] or ''}",
                f"- approval_required: {candidate.get('approval_required')}",
                f"- vetoed: {veto.get('vetoed')}",
                f"- risk_status: {candidate.get('risk_status')}",
                f"- paper_trade_rows: {paper_count}",
                f"- tax_lot_rows: {tax_lot_count}",
            ]
        )
        for item in candidate.get("candidates", []):
            lines.append(f"- candidate: {item.get('symbol')} weight={item.get('target_weight')} score={item.get('adjusted_score')}")
    lines.extend(
        [
            "",
            "## Paper Operation Gate",
            "",
            f"- status: {paper_gate['status']}",
            f"- paper_weeks: {paper_gate['paper_weeks']}",
            f"- missing_decision_paper: {paper_gate['missing_decision_paper']}",
            f"- llm_audit_rows: {paper_gate['llm_audit_rows']}",
            f"- no_trade_rows: {paper_gate['no_trade_rows']}",
            f"- event_veto_rows: {paper_gate['event_veto_rows']}",
            f"- missing_weekly_logs: {paper_gate['missing_weekly_logs']}",
            f"- missing_logs: {json.dumps(paper_gate['missing_logs'], ensure_ascii=False, sort_keys=True)}",
            "",
            "### Weekly Ledger",
            "",
            "| snapshot_date | decision_id | snapshot_id | complete | missing |",
            "|---|---:|---:|---|---|",
        ]
    )
    for week in paper_gate["weeks"]:
        missing = ", ".join(str(item) for item in week["missing"]) or "-"
        lines.append(
            f"| {week['snapshot_date']} | {week['decision_id']} | {week['snapshot_id']} | {str(week['complete']).lower()} | {missing} |"
        )
    lines.append("")

    output_dir = options.output_dir or Path("rencrow-data/reports")
    output_dir.mkdir(parents=True, exist_ok=True)
    output_path = output_dir / f"audit_snapshot_{options.snapshot_id}.md"
    output_path.write_text("\n".join(lines), encoding="utf-8")
    return {
        "snapshot_id": options.snapshot_id,
        "snapshot_date": snapshot_date,
        "decision_id": decision_id,
        "output_path": str(output_path),
        "fetch_failures_total": fetch_failures,
        "fetch_partials_total": fetch_partials,
        "quality_blockers": quality_blockers,
        "risk_status": None if risk is None else risk["status"],
        "paper_latest": options.paper_latest,
        "paper_gate": paper_gate,
    }
