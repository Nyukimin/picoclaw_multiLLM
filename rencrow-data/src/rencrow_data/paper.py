from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class PaperTradeOptions:
    decision_id: int
    approval_file: Path
    fill_model: str = "close_next_week"
    capital: float = 1_000_000.0
    cost_bps: float = 10.0


def _load_approval(path: Path) -> dict[str, object]:
    if not path.exists():
        raise FileNotFoundError(path)
    text = path.read_text(encoding="utf-8").strip()
    if not text:
        return {}
    if text.startswith("{"):
        return json.loads(text)
    approval: dict[str, object] = {}
    for raw_line in text.splitlines():
        line = raw_line.split("#", 1)[0].strip()
        if not line or ":" not in line:
            continue
        key, value = line.split(":", 1)
        value = value.strip()
        if value.lower() in {"true", "false"}:
            parsed: object = value.lower() == "true"
        elif value == "":
            parsed = ""
        elif value.startswith(("'", '"')) and value.endswith(("'", '"')):
            parsed = value[1:-1]
        else:
            try:
                parsed = int(value)
            except ValueError:
                parsed = value
        approval[key.strip()] = parsed
    return approval


def _decision(con, decision_id: int):
    row = con.execute("SELECT * FROM decision_log WHERE decision_id=?", (decision_id,)).fetchone()
    if row is None:
        raise ValueError(f"decision not found: {decision_id}")
    return row


def _require_approval_metadata(approval: dict[str, object]) -> None:
    missing = [key for key in ("approver", "approved_at", "approval_reason") if not str(approval.get(key) or "").strip()]
    if missing:
        raise PermissionError(f"approval file is missing required metadata: {', '.join(missing)}")


def _feature_price(con, instrument_id: int, week_end: str, fill_model: str) -> float | None:
    if fill_model == "close_next_week":
        row = con.execute(
            """
            SELECT close_adj_jpy
              FROM feature_weekly
             WHERE instrument_id=? AND week_end>?
             ORDER BY week_end
             LIMIT 1
            """,
            (instrument_id, week_end),
        ).fetchone()
        if row is not None and row["close_adj_jpy"] is not None:
            return float(row["close_adj_jpy"])
    row = con.execute(
        """
        SELECT close_adj_jpy
          FROM feature_weekly
         WHERE instrument_id=? AND week_end<=?
         ORDER BY week_end DESC
         LIMIT 1
        """,
        (instrument_id, week_end),
    ).fetchone()
    return None if row is None or row["close_adj_jpy"] is None else float(row["close_adj_jpy"])


def run_paper_trade(con, options: PaperTradeOptions) -> dict[str, object]:
    approval = _load_approval(options.approval_file)
    if int(approval.get("decision_id", -1)) != options.decision_id:
        raise ValueError("approval decision_id does not match")
    if not approval.get("approved"):
        raise PermissionError("approval file is present but approved=false")
    _require_approval_metadata(approval)
    decision = _decision(con, options.decision_id)
    snapshot_id = decision["snapshot_id"]
    con.execute(
        """
        UPDATE decision_log
           SET approved=1, approver=?, approved_at=?
         WHERE decision_id=?
        """,
        (approval.get("approver") or "", approval.get("approved_at") or "", options.decision_id),
    )
    candidate = json.loads(decision["candidate_json"] or "{}")
    veto = json.loads(decision["veto_json"] or "{}")
    candidates = candidate.get("candidates", [])
    if veto.get("vetoed") or not candidates:
        con.execute(
            """
            INSERT INTO paper_trade_log(snapshot_id, decision_id, instrument_id, side, quantity, decision_price, simulated_fill_price, cost_bps, status)
            VALUES (?, ?, NULL, 'HOLD', 0, NULL, NULL, ?, ?)
            """,
            (snapshot_id, options.decision_id, options.cost_bps, "vetoed" if veto.get("vetoed") else "no_candidate"),
        )
        con.commit()
        paper_trade_id = int(con.execute("SELECT last_insert_rowid()").fetchone()[0])
        return {
            "paper_trade_id": paper_trade_id,
            "snapshot_id": snapshot_id,
            "decision_id": options.decision_id,
            "status": "vetoed" if veto.get("vetoed") else "no_candidate",
            "trades": [],
            "tca": {"fill_model": options.fill_model, "cost_bps": options.cost_bps},
        }

    trades: list[dict[str, object]] = []
    total_slippage = 0.0
    total_cost = 0.0
    for item in candidates:
        instrument_id = int(item["instrument_id"])
        target_weight = float(item.get("target_weight") or 0.0)
        decision_price = _feature_price(con, instrument_id, candidate["week_end"], "decision_close")
        fill_price = _feature_price(con, instrument_id, candidate["week_end"], options.fill_model)
        if fill_price is None or fill_price <= 0:
            raise ValueError(f"fill price unavailable for instrument_id={instrument_id}")
        quantity = options.capital * target_weight / fill_price
        side = "BUY" if quantity > 0 else "HOLD"
        status = "simulated" if quantity > 0 else "zero_weight"
        notional = quantity * fill_price
        estimated_cost = notional * options.cost_bps / 10000.0
        slippage = None if decision_price in (None, 0) else fill_price / decision_price - 1.0
        if slippage is not None:
            total_slippage += abs(slippage) * notional
        total_cost += estimated_cost
        con.execute(
            """
            INSERT INTO paper_trade_log(snapshot_id, decision_id, instrument_id, side, quantity, decision_price, simulated_fill_price, cost_bps, status)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (snapshot_id, options.decision_id, instrument_id, side, quantity, decision_price, fill_price, options.cost_bps, status),
        )
        paper_trade_id = int(con.execute("SELECT last_insert_rowid()").fetchone()[0])
        if side == "BUY":
            con.execute(
                """
                INSERT INTO tax_lot_log(
                  account_scope, instrument_id, acquired_date, quantity, acquisition_price,
                  disposed_date, disposal_price, realized_pnl, source_order_id
                )
                VALUES ('taxable', ?, ?, ?, ?, NULL, NULL, NULL, ?)
                """,
                (instrument_id, candidate["week_end"], quantity, fill_price, paper_trade_id),
            )
        trades.append(
            {
                "paper_trade_id": paper_trade_id,
                "snapshot_id": snapshot_id,
                "instrument_id": instrument_id,
                "symbol": item.get("symbol"),
                "side": side,
                "quantity": quantity,
                "decision_price": decision_price,
                "simulated_fill_price": fill_price,
                "target_weight": target_weight,
                "notional": notional,
                "estimated_cost": estimated_cost,
                "slippage": slippage,
                "status": status,
            }
        )
    con.commit()
    return {
        "decision_id": options.decision_id,
        "snapshot_id": snapshot_id,
        "status": "simulated",
        "trades": trades,
        "tca": {
            "fill_model": options.fill_model,
            "cost_bps": options.cost_bps,
            "capital": options.capital,
            "trade_count": len(trades),
            "estimated_total_cost": total_cost,
            "notional_weighted_abs_slippage": 0.0 if options.capital == 0 else total_slippage / options.capital,
        },
    }
