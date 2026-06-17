from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import date, timedelta

from .timeutil import utcnow_iso


@dataclass(frozen=True)
class RiskOptions:
    snapshot_id: str
    strategy_id: str
    decision_id: str | None = None
    config: dict[str, object] | None = None


def _json(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def _latest_backtest_id(con, snapshot_id: str, strategy_id: str) -> str | None:
    row = con.execute(
        """
        SELECT backtest_id
          FROM backtest_run
         WHERE snapshot_id=? AND strategy_id=? AND status='success'
         ORDER BY created_at DESC
         LIMIT 1
        """,
        (snapshot_id, strategy_id),
    ).fetchone()
    return None if row is None else row["backtest_id"]


def _metrics(con, backtest_id: str) -> dict[str, float]:
    return {
        row["metric_name"]: float(row["metric_value"])
        for row in con.execute(
            "SELECT metric_name, metric_value FROM backtest_metric WHERE backtest_id=?",
            (backtest_id,),
        )
    }


def _snapshot_date(con, snapshot_id: str) -> str:
    row = con.execute("SELECT snapshot_date FROM snapshot_registry WHERE snapshot_id=?", (snapshot_id,)).fetchone()
    if row is None:
        raise ValueError(f"snapshot not found: {snapshot_id}")
    return row["snapshot_date"]


def _data_quality_blockers(con, snapshot_date: str) -> int:
    row = con.execute(
        """
        SELECT COUNT(*) AS count
          FROM data_quality_check
         WHERE check_date=? AND severity='blocker' AND status!='pass'
        """,
        (snapshot_date,),
    ).fetchone()
    return int(row["count"] or 0)


def _event_blockers(con, snapshot_date: str, lookback_days: int) -> int:
    end = date.fromisoformat(snapshot_date)
    start = (end - timedelta(days=max(lookback_days, 0))).isoformat()
    row = con.execute(
        """
        SELECT COUNT(*) AS count
          FROM event_log
         WHERE date(event_ts) BETWEEN date(?) AND date(?)
           AND resolved_at IS NULL
           AND level IN ('stop', 'kill')
        """,
        (start, snapshot_date),
    ).fetchone()
    return int(row["count"] or 0)


def run_risk_check(con, options: RiskOptions) -> dict[str, object]:
    config = options.config or {}
    snapshot_date = _snapshot_date(con, options.snapshot_id)
    backtest_id = _latest_backtest_id(con, options.snapshot_id, options.strategy_id)
    if backtest_id is None:
        raise ValueError(f"successful backtest not found for strategy={options.strategy_id} snapshot={options.snapshot_id}")
    metrics = _metrics(con, backtest_id)

    max_dd_limit = float(config.get("max_drawdown_limit", 0.25))
    weekly_loss_limit = float(config.get("weekly_loss_limit", 0.08))
    vol_limit = float(config.get("annualized_volatility_limit", 0.30))
    turnover_warning_limit = float(config.get("turnover_warning_limit", 0.50))
    event_threshold = float(config.get("event_risk_stop_threshold", 0.9))
    event_lookback_days = int(config.get("event_lookback_days", 7))

    max_dd_check = "fail" if metrics.get("max_dd", 0.0) < -max_dd_limit else "pass"
    weekly_loss_check = "fail" if metrics.get("worst_week", 0.0) < -weekly_loss_limit else "pass"
    volatility_check = "fail" if metrics.get("annualized_volatility", 0.0) > vol_limit else "pass"
    concentration_check = "warning" if metrics.get("turnover", 0.0) > turnover_warning_limit else "pass"

    quality_blockers = _data_quality_blockers(con, snapshot_date)
    event_blockers = _event_blockers(con, snapshot_date, event_lookback_days)
    event_check = "fail" if quality_blockers or event_blockers else "pass"

    status = "pass"
    if concentration_check == "warning":
        status = "reduce"
    if max_dd_check == "fail" or weekly_loss_check == "fail" or volatility_check == "fail" or event_check == "fail":
        status = "stop"
    if event_blockers:
        status = "kill_switch"

    detail = {
        "snapshot_date": snapshot_date,
        "backtest_id": backtest_id,
        "metrics": metrics,
        "limits": {
            "max_drawdown_limit": max_dd_limit,
            "weekly_loss_limit": weekly_loss_limit,
            "annualized_volatility_limit": vol_limit,
            "turnover_warning_limit": turnover_warning_limit,
            "event_risk_stop_threshold": event_threshold,
            "event_lookback_days": event_lookback_days,
        },
        "quality_blockers": quality_blockers,
        "event_blockers": event_blockers,
    }
    risk_check_id = f"risk-{utcnow_iso()}"
    con.execute(
        """
        INSERT INTO risk_check_result(
          risk_check_id, snapshot_id, strategy_id, decision_id, status, max_dd_check,
          weekly_loss_check, concentration_check, volatility_check, event_check, detail_json
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (
            risk_check_id,
            options.snapshot_id,
            options.strategy_id,
            options.decision_id,
            status,
            max_dd_check,
            weekly_loss_check,
            concentration_check,
            volatility_check,
            event_check,
            _json(detail),
        ),
    )
    con.commit()
    return {
        "risk_check_id": risk_check_id,
        "snapshot_id": options.snapshot_id,
        "strategy_id": options.strategy_id,
        "decision_id": options.decision_id,
        "status": status,
        "max_dd_check": max_dd_check,
        "weekly_loss_check": weekly_loss_check,
        "concentration_check": concentration_check,
        "volatility_check": volatility_check,
        "event_check": event_check,
        "detail": detail,
    }


def exit_code(result: dict[str, object]) -> int:
    return 3 if result.get("status") in {"stop", "kill_switch"} else 0
