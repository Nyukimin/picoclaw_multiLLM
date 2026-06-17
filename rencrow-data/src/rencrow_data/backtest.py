from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from datetime import date
from pathlib import Path

from .timeutil import utcnow_iso


JAPAN_TAX_RATE = 0.20315


@dataclass(frozen=True)
class BacktestOptions:
    snapshot_id: str
    strategy_id: str
    start: date | None = None
    end: date | None = None
    cost_bps: float = 10.0
    slippage_bps: float = 0.0
    tax_mode: str = "none"
    mode: str = "full"
    output_dir: Path | None = None
    symbols: tuple[str, ...] = ()


def _json(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def _load_strategy(con, strategy_id: str) -> dict[str, object]:
    row = con.execute(
        "SELECT config_json FROM strategy_version WHERE strategy_id=? AND active=1",
        (strategy_id,),
    ).fetchone()
    if row is None:
        raise KeyError(f"active strategy config not found: {strategy_id}")
    return json.loads(row["config_json"] or "{}")


def _instrument_ids(con, symbols: tuple[str, ...]) -> dict[int, str]:
    if not symbols:
        raise ValueError("strategy universe is empty")
    rows = con.execute(
        f"""
        SELECT instrument_id, symbol
          FROM instruments
         WHERE symbol IN ({",".join("?" for _ in symbols)}) AND active=1
        """,
        symbols,
    ).fetchall()
    found = {int(row["instrument_id"]): row["symbol"] for row in rows}
    found_symbols = set(found.values())
    missing = [symbol for symbol in symbols if symbol not in found_symbols]
    if missing:
        raise ValueError(f"strategy universe symbols are missing from instruments: {', '.join(missing)}")
    return found


def _load_features(con, iids: tuple[int, ...], start: date | None, end: date | None) -> dict[str, dict[int, dict[str, float | None]]]:
    clauses = [f"instrument_id IN ({','.join('?' for _ in iids)})"]
    params: list[object] = list(iids)
    if start is not None:
        clauses.append("week_end>=?")
        params.append(start.isoformat())
    if end is not None:
        clauses.append("week_end<=?")
        params.append(end.isoformat())
    rows = con.execute(
        f"""
        SELECT instrument_id, week_end, ret_1w, ret_12w, ret_12w_skip1, vol_12w, drawdown_26w, close_adj_jpy, event_risk_score
          FROM feature_weekly
         WHERE {' AND '.join(clauses)}
         ORDER BY week_end, instrument_id
        """,
        params,
    ).fetchall()
    by_week: dict[str, dict[int, dict[str, float | None]]] = {}
    for row in rows:
        by_week.setdefault(row["week_end"], {})[int(row["instrument_id"])] = {
            "ret_1w": row["ret_1w"],
            "ret_12w": row["ret_12w"],
            "ret_12w_skip1": row["ret_12w_skip1"],
            "vol_12w": row["vol_12w"],
            "drawdown_26w": row["drawdown_26w"],
            "close_adj_jpy": row["close_adj_jpy"],
            "event_risk_score": row["event_risk_score"],
        }
    return by_week


def _score(feature: dict[str, float | None], config: dict[str, object]) -> float | None:
    momentum = feature.get("ret_12w_skip1")
    if momentum is None:
        momentum = feature.get("ret_12w")
    if momentum is None:
        return None
    vol = feature.get("vol_12w") or 0.0
    drawdown = feature.get("drawdown_26w") or 0.0
    return float(momentum) - float(config.get("volatility_penalty", 0.5)) * float(vol) + float(config.get("drawdown_penalty", 0.25)) * float(drawdown)


def _max_drawdown(equity: list[float]) -> float:
    peak = equity[0] if equity else 1.0
    worst = 0.0
    for value in equity:
        peak = max(peak, value)
        if peak:
            worst = min(worst, value / peak - 1.0)
    return worst


def _std(values: list[float]) -> float:
    if len(values) < 2:
        return 0.0
    mean = sum(values) / len(values)
    return math.sqrt(sum((v - mean) ** 2 for v in values) / (len(values) - 1))


def _metrics(returns: list[float], equity_curve: list[dict[str, object]], turnover: float, cost_drag: float, tax_drag: float) -> dict[str, float]:
    weeks = len(returns)
    final_equity = float(equity_curve[-1]["equity"]) if equity_curve else 1.0
    cagr = final_equity ** (52.0 / weeks) - 1.0 if weeks > 0 and final_equity > 0 else 0.0
    vol = _std(returns) * math.sqrt(52)
    mean = sum(returns) / weeks if weeks else 0.0
    downside = _std([ret for ret in returns if ret < 0])
    return {
        "final_equity": final_equity,
        "cagr": cagr,
        "annualized_volatility": vol,
        "sharpe": 0.0 if vol == 0 else mean * 52 / vol,
        "sortino": 0.0 if downside == 0 else mean * 52 / (downside * math.sqrt(52)),
        "max_dd": _max_drawdown([float(row["equity"]) for row in equity_curve]),
        "turnover": turnover / weeks if weeks else 0.0,
        "hit_rate": sum(1 for ret in returns if ret > 0) / weeks if weeks else 0.0,
        "exposure": sum(1 for row in equity_curve if row.get("symbol")) / len(equity_curve) if equity_curve else 0.0,
        "cost_drag": cost_drag,
        "tax_drag": tax_drag,
        "worst_week": min(returns) if returns else 0.0,
    }


def _write_csv(path: Path, rows: list[dict[str, object]], fieldnames: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


def run_weekly_rotation_backtest(con, options: BacktestOptions) -> dict[str, object]:
    config = _load_strategy(con, options.strategy_id)
    universe = options.symbols or tuple(str(symbol) for symbol in config.get("universe", []))
    iid_to_symbol = _instrument_ids(con, universe)
    by_week = _load_features(con, tuple(iid_to_symbol), options.start, options.end)
    weeks = sorted(by_week)
    if len(weeks) < 14:
        raise ValueError("not enough weekly feature rows for backtest")

    cost_rate = (options.cost_bps + options.slippage_bps) / 10000.0
    score_min = float(config.get("score_min", -999.0))
    cash_proxy = str(config.get("cash_proxy", ""))
    event_veto_threshold = float(config.get("event_veto_threshold", 0.9))
    cash_iid = next((iid for iid, symbol in iid_to_symbol.items() if symbol == cash_proxy), None)
    equity = 1.0
    selected_iid: int | None = None
    entry_equity: float | None = None
    signal_iid: int | None = None
    returns: list[float] = []
    turnover = 0.0
    cost_drag = 0.0
    tax_drag = 0.0
    equity_curve: list[dict[str, object]] = []
    trades: list[dict[str, object]] = []

    for idx, week in enumerate(weeks):
        start_equity = equity
        feature_by_iid = by_week[week]
        period_return = 0.0
        gross_return = 0.0
        if idx > 0 and selected_iid is not None:
            selected_feature = feature_by_iid.get(selected_iid)
            if selected_feature is not None and selected_feature.get("ret_1w") is not None:
                gross_return = float(selected_feature["ret_1w"])
                equity *= 1.0 + gross_return
        if idx > 0 and selected_iid != signal_iid:
            if options.tax_mode == "approx_jp_taxable" and selected_iid is not None and entry_equity is not None:
                tax_amount = max(equity - entry_equity, 0.0) * JAPAN_TAX_RATE
                equity -= tax_amount
                tax_drag += 0.0 if start_equity == 0 else tax_amount / start_equity
            cost_amount = equity * cost_rate
            equity -= cost_amount
            cost_drag += 0.0 if start_equity == 0 else cost_amount / start_equity
            turnover += 1.0
            trades.append(
                {
                    "week_end": week,
                    "from_symbol": "" if selected_iid is None else iid_to_symbol[selected_iid],
                    "to_symbol": "" if signal_iid is None else iid_to_symbol[signal_iid],
                    "cost_rate": cost_rate,
                }
            )
            entry_equity = equity if signal_iid is not None else None
        if idx > 0:
            period_return = 0.0 if start_equity == 0 else equity / start_equity - 1.0
            returns.append(period_return)
        selected_iid = signal_iid
        if idx == 0 and selected_iid is not None:
            entry_equity = equity

        event_vetoed = any(float(feature.get("event_risk_score") or 0.0) >= event_veto_threshold for feature in feature_by_iid.values())
        best_iid: int | None = None
        best_score: float | None = None
        if event_vetoed and cash_iid is not None:
            best_iid = cash_iid
            best_score = None
        else:
            for iid, feature in feature_by_iid.items():
                score = _score(feature, config)
                if score is None:
                    continue
                if best_score is None or score > best_score:
                    best_score = score
                    best_iid = iid
            if best_score is None or best_score < score_min:
                best_iid = cash_iid
        signal_iid = best_iid

        equity_curve.append(
            {
                "week_end": week,
                "symbol": "" if selected_iid is None else iid_to_symbol[selected_iid],
                "signal_symbol": "" if signal_iid is None else iid_to_symbol[signal_iid],
                "event_vetoed": int(event_vetoed),
                "period_return": period_return,
                "equity": equity,
            }
        )

    metrics = _metrics(returns, equity_curve, turnover, cost_drag, tax_drag)
    backtest_id = f"backtest-{utcnow_iso()}"
    output_dir = options.output_dir or Path("rencrow-data/data/backtests")
    equity_path = output_dir / f"{backtest_id}_equity.csv"
    trades_path = output_dir / f"{backtest_id}_trades.csv"
    _write_csv(equity_path, equity_curve, ["week_end", "symbol", "signal_symbol", "event_vetoed", "period_return", "equity"])
    _write_csv(trades_path, trades, ["week_end", "from_symbol", "to_symbol", "cost_rate"])
    result = {
        "backtest_id": backtest_id,
        "strategy_id": options.strategy_id,
        "snapshot_id": options.snapshot_id,
        "start_date": weeks[0],
        "end_date": weeks[-1],
        "weeks": len(returns),
        "equity_curve_path": str(equity_path),
        "trades_path": str(trades_path),
        "metrics": metrics,
        "universe": list(universe),
    }
    con.execute(
        """
        INSERT INTO backtest_run(backtest_id, strategy_id, snapshot_id, start_date, end_date, mode, cost_bps, slippage_bps, tax_mode, status, result_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'success', ?)
        """,
        (
            backtest_id,
            options.strategy_id,
            options.snapshot_id,
            weeks[0],
            weeks[-1],
            options.mode,
            options.cost_bps,
            options.slippage_bps,
            options.tax_mode,
            _json(result),
        ),
    )
    for name, value in metrics.items():
        con.execute(
            """
            INSERT INTO backtest_metric(backtest_id, split_name, metric_name, metric_value)
            VALUES (?, 'full', ?, ?)
            """,
            (backtest_id, name, float(value)),
        )
    con.commit()
    return result
