from __future__ import annotations

import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from rencrow_data import db


class InitDBTest(unittest.TestCase):
    def test_init_db_creates_all_tables_and_blocks_nisa_scope(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            con = db.connect(tmp_path / "rencrow.db")
            db.init_schema(con)
            tables = {
                row[0]
                for row in con.execute(
                    "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"
                ).fetchall()
            }
            expected = {
                "instruments",
                "source_fetch_log",
                "price_raw",
                "corporate_action",
                "macro_series",
                "economic_calendar",
                "etf_holding_snapshot",
                "feature_weekly",
                "event_log",
                "snapshot_registry",
                "decision_log",
                "paper_trade_log",
                "order_log",
                "tax_lot_log",
            }
            self.assertLessEqual(expected, tables)

            db.upsert_instruments(
                con,
                [
                    {
                        "symbol": "1306.T",
                        "name": "TOPIX ETF",
                        "asset_type": "ETF",
                        "venue": "TSE",
                        "currency": "JPY",
                        "first_date": "2001-01-01",
                    }
                ],
            )
            self.assertEqual(con.execute("SELECT COUNT(*) FROM instruments").fetchone()[0], 1)
            with self.assertRaises(sqlite3.IntegrityError):
                con.execute("INSERT INTO decision_log(decision_date, account_scope) VALUES ('2026-01-01', 'nisa')")


if __name__ == "__main__":
    unittest.main()
