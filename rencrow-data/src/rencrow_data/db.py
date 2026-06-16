from __future__ import annotations

import sqlite3
from pathlib import Path
from typing import Iterable

from .timeutil import utcnow_iso


SCHEMA_SQL = """
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS instruments (
  instrument_id INTEGER PRIMARY KEY,
  symbol TEXT NOT NULL,
  name TEXT,
  asset_type TEXT,
  venue TEXT,
  currency TEXT,
  timezone TEXT,
  active INTEGER DEFAULT 1,
  first_date TEXT,
  last_date TEXT,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(symbol, venue, first_date)
);

CREATE TABLE IF NOT EXISTS source_fetch_log (
  fetch_id INTEGER PRIMARY KEY,
  source_name TEXT NOT NULL,
  endpoint TEXT,
  requested_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL,
  http_status INTEGER,
  rows_fetched INTEGER,
  checksum TEXT,
  retry_count INTEGER DEFAULT 0,
  error_message TEXT,
  raw_cache_path TEXT
);

CREATE TABLE IF NOT EXISTS price_raw (
  instrument_id INTEGER NOT NULL,
  trade_date TEXT NOT NULL,
  open REAL,
  high REAL,
  low REAL,
  close REAL,
  adj_close REAL,
  volume REAL,
  source_name TEXT NOT NULL,
  fetch_id INTEGER,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (instrument_id, trade_date, source_name)
);

CREATE TABLE IF NOT EXISTS corporate_action (
  instrument_id INTEGER NOT NULL,
  action_date TEXT NOT NULL,
  action_type TEXT NOT NULL,
  value REAL,
  currency TEXT,
  source_name TEXT NOT NULL,
  fetch_id INTEGER,
  context_json TEXT,
  PRIMARY KEY (instrument_id, action_date, action_type, source_name)
);

CREATE TABLE IF NOT EXISTS macro_series (
  series_code TEXT NOT NULL,
  obs_date TEXT NOT NULL,
  value REAL,
  vintage_date TEXT NOT NULL DEFAULT '',
  release_date TEXT,
  source_name TEXT NOT NULL,
  fetch_id INTEGER,
  unit TEXT,
  PRIMARY KEY (series_code, obs_date, vintage_date, source_name)
);

CREATE TABLE IF NOT EXISTS economic_calendar (
  event_id INTEGER PRIMARY KEY,
  event_date TEXT NOT NULL,
  event_time_utc TEXT,
  country TEXT,
  category TEXT,
  event_name TEXT NOT NULL,
  source_name TEXT NOT NULL,
  importance TEXT,
  last_checked_at TEXT,
  context_json TEXT
);

CREATE TABLE IF NOT EXISTS etf_holding_snapshot (
  instrument_id INTEGER NOT NULL,
  snapshot_date TEXT NOT NULL,
  constituent_code TEXT NOT NULL,
  constituent_name TEXT,
  weight REAL,
  quantity REAL,
  sector TEXT,
  source_name TEXT NOT NULL,
  fetch_id INTEGER,
  PRIMARY KEY (instrument_id, snapshot_date, constituent_code, source_name)
);

CREATE TABLE IF NOT EXISTS feature_weekly (
  instrument_id INTEGER NOT NULL,
  week_end TEXT NOT NULL,
  close_adj_jpy REAL,
  ret_1w REAL,
  ret_4w REAL,
  ret_12w REAL,
  vol_12w REAL,
  drawdown_26w REAL,
  ma_4w_gap REAL,
  ma_12w_gap REAL,
  volume_change_4w REAL,
  fx_ret_1w REAL,
  us10y_change_1w REAL,
  boj_flag INTEGER DEFAULT 0,
  cpi_flag INTEGER DEFAULT 0,
  fomc_flag INTEGER DEFAULT 0,
  employment_flag INTEGER DEFAULT 0,
  holdings_turnover REAL,
  event_risk_score REAL DEFAULT 0,
  source_snapshot_id INTEGER,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (instrument_id, week_end)
);

CREATE TABLE IF NOT EXISTS event_log (
  event_id INTEGER PRIMARY KEY,
  event_ts TEXT NOT NULL,
  scope TEXT NOT NULL,
  level TEXT NOT NULL,
  reason TEXT NOT NULL,
  value REAL,
  event_risk_score REAL,
  context_json TEXT,
  resolved_at TEXT,
  resolution_note TEXT
);

CREATE TABLE IF NOT EXISTS snapshot_registry (
  snapshot_id INTEGER PRIMARY KEY,
  snapshot_date TEXT NOT NULL UNIQUE,
  snapshot_path TEXT,
  db_hash TEXT,
  features_hash TEXT,
  source_summary_json TEXT,
  data_start_date TEXT,
  data_end_date TEXT,
  missing_rate REAL,
  event_state_json TEXT,
  status TEXT,
  notes TEXT,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS decision_log (
  decision_id INTEGER PRIMARY KEY,
  snapshot_id INTEGER,
  decision_date TEXT,
  account_scope TEXT CHECK(account_scope IS NULL OR account_scope IN ('taxable', 'paper')),
  strategy_name TEXT,
  candidate_json TEXT,
  veto_json TEXT,
  approved INTEGER DEFAULT 0,
  approver TEXT,
  approved_at TEXT,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS paper_trade_log (
  paper_trade_id INTEGER PRIMARY KEY,
  decision_id INTEGER,
  instrument_id INTEGER,
  side TEXT,
  quantity REAL,
  decision_price REAL,
  simulated_fill_price REAL,
  cost_bps REAL,
  status TEXT,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS order_log (
  order_id INTEGER PRIMARY KEY,
  decision_id INTEGER,
  broker_order_id TEXT,
  instrument_id INTEGER,
  side TEXT,
  order_type TEXT,
  quantity REAL,
  limit_price REAL,
  status TEXT,
  submitted_at TEXT,
  filled_at TEXT,
  fill_price REAL,
  error_message TEXT
);

CREATE TABLE IF NOT EXISTS tax_lot_log (
  tax_lot_id INTEGER PRIMARY KEY,
  account_scope TEXT CHECK(account_scope IS NULL OR account_scope = 'taxable'),
  instrument_id INTEGER,
  acquired_date TEXT,
  quantity REAL,
  acquisition_price REAL,
  disposed_date TEXT,
  disposal_price REAL,
  realized_pnl REAL,
  source_order_id INTEGER,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_instruments_symbol ON instruments(symbol);
CREATE INDEX IF NOT EXISTS idx_instruments_asset_type ON instruments(asset_type);
CREATE INDEX IF NOT EXISTS idx_instruments_active ON instruments(active);
CREATE INDEX IF NOT EXISTS idx_fetch_source_requested ON source_fetch_log(source_name, requested_at);
CREATE INDEX IF NOT EXISTS idx_fetch_status ON source_fetch_log(status);
CREATE INDEX IF NOT EXISTS idx_fetch_finished ON source_fetch_log(finished_at);
CREATE INDEX IF NOT EXISTS idx_price_trade_date ON price_raw(trade_date);
CREATE INDEX IF NOT EXISTS idx_price_fetch ON price_raw(fetch_id);
CREATE INDEX IF NOT EXISTS idx_macro_series_obs ON macro_series(series_code, obs_date);
CREATE INDEX IF NOT EXISTS idx_macro_release ON macro_series(release_date);
CREATE INDEX IF NOT EXISTS idx_calendar_date ON economic_calendar(event_date);
CREATE INDEX IF NOT EXISTS idx_calendar_category_importance ON economic_calendar(category, importance);
CREATE INDEX IF NOT EXISTS idx_feature_week_end ON feature_weekly(week_end);
CREATE INDEX IF NOT EXISTS idx_feature_risk ON feature_weekly(event_risk_score);
CREATE INDEX IF NOT EXISTS idx_event_ts ON event_log(event_ts);
CREATE INDEX IF NOT EXISTS idx_event_level ON event_log(level);
CREATE INDEX IF NOT EXISTS idx_event_reason ON event_log(reason);
CREATE INDEX IF NOT EXISTS idx_event_resolved ON event_log(resolved_at);
CREATE INDEX IF NOT EXISTS idx_snapshot_date ON snapshot_registry(snapshot_date);
CREATE INDEX IF NOT EXISTS idx_snapshot_status ON snapshot_registry(status);
CREATE INDEX IF NOT EXISTS idx_decision_date ON decision_log(decision_date);
CREATE INDEX IF NOT EXISTS idx_decision_snapshot ON decision_log(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_decision_account ON decision_log(account_scope);
"""


def connect(db_path: str | Path) -> sqlite3.Connection:
    path = Path(db_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    con = sqlite3.connect(path)
    con.row_factory = sqlite3.Row
    con.execute("PRAGMA journal_mode=WAL")
    con.execute("PRAGMA foreign_keys=ON")
    return con


def init_schema(con: sqlite3.Connection) -> None:
    con.executescript(SCHEMA_SQL)
    con.commit()


def upsert_instrument(con: sqlite3.Connection, item: dict) -> int:
    now = utcnow_iso()
    con.execute(
        """
        INSERT INTO instruments(symbol, name, asset_type, venue, currency, timezone, active, first_date, last_date, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(symbol, venue, first_date) DO UPDATE SET
          name=excluded.name,
          asset_type=excluded.asset_type,
          currency=excluded.currency,
          timezone=excluded.timezone,
          active=excluded.active,
          last_date=excluded.last_date,
          updated_at=excluded.updated_at
        """,
        (
            item["symbol"],
            item.get("name"),
            item.get("asset_type", "ETF"),
            item.get("venue", "TSE"),
            item.get("currency", "JPY"),
            item.get("timezone", "Asia/Tokyo"),
            int(item.get("active", 1)),
            item.get("first_date", ""),
            item.get("last_date"),
            now,
            now,
        ),
    )
    row = con.execute(
        "SELECT instrument_id FROM instruments WHERE symbol=? AND venue=? AND first_date=?",
        (item["symbol"], item.get("venue", "TSE"), item.get("first_date", "")),
    ).fetchone()
    if row is None:
        raise RuntimeError(f"instrument upsert failed: {item['symbol']}")
    return int(row["instrument_id"])


def upsert_instruments(con: sqlite3.Connection, items: Iterable[dict]) -> int:
    count = 0
    for item in items:
        upsert_instrument(con, item)
        count += 1
    con.commit()
    return count


def instrument_id(con: sqlite3.Connection, symbol: str, venue: str | None = None) -> int:
    if venue:
        row = con.execute(
            "SELECT instrument_id FROM instruments WHERE symbol=? AND venue=? ORDER BY active DESC, instrument_id DESC LIMIT 1",
            (symbol, venue),
        ).fetchone()
    else:
        row = con.execute(
            "SELECT instrument_id FROM instruments WHERE symbol=? ORDER BY active DESC, instrument_id DESC LIMIT 1",
            (symbol,),
        ).fetchone()
    if row is None:
        raise KeyError(f"unknown instrument: {symbol}")
    return int(row["instrument_id"])


def start_fetch(con: sqlite3.Connection, source_name: str, endpoint: str) -> int:
    con.execute(
        "INSERT INTO source_fetch_log(source_name, endpoint, requested_at, status) VALUES (?, ?, ?, 'running')",
        (source_name, endpoint, utcnow_iso()),
    )
    con.commit()
    return int(con.execute("SELECT last_insert_rowid()").fetchone()[0])


def finish_fetch(
    con: sqlite3.Connection,
    fetch_id: int,
    status: str,
    rows_fetched: int = 0,
    http_status: int | None = None,
    checksum: str | None = None,
    retry_count: int = 0,
    error_message: str | None = None,
    raw_cache_path: str | None = None,
) -> None:
    con.execute(
        """
        UPDATE source_fetch_log
        SET finished_at=?, status=?, http_status=?, rows_fetched=?, checksum=?, retry_count=?, error_message=?, raw_cache_path=?
        WHERE fetch_id=?
        """,
        (utcnow_iso(), status, http_status, rows_fetched, checksum, retry_count, error_message, raw_cache_path, fetch_id),
    )
    con.commit()


def log_event(
    con: sqlite3.Connection,
    scope: str,
    level: str,
    reason: str,
    value: float | None = None,
    event_risk_score: float | None = None,
    context_json: str | None = None,
) -> None:
    con.execute(
        """
        INSERT INTO event_log(event_ts, scope, level, reason, value, event_risk_score, context_json)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        (utcnow_iso(), scope, level, reason, value, event_risk_score, context_json),
    )
    con.commit()
