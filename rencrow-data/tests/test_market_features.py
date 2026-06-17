from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from rencrow_data import db
from rencrow_data.features import build_features
from rencrow_data.market import save_market_csv


class MarketFeatureTest(unittest.TestCase):
    def test_market_ingest_logs_price_revision_and_feature_uses_adjustment(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            price_csv = tmp_path / "prices.csv"
            price_csv.write_text(
                "\n".join(
                    [
                        "date,open,high,low,close,adj_close,volume,dividend,split",
                        "2026-01-02,100,101,99,100,50,1000,0,2",
                        "2026-01-09,102,103,101,102,102,1000,0,1",
                        "2026-01-16,104,105,103,104,104,1000,0,1",
                        "2026-01-23,106,107,105,106,106,1000,0,1",
                        "2026-01-30,108,109,107,108,108,1000,0,1",
                    ]
                ),
                encoding="utf-8",
            )
            con = db.connect(tmp_path / "rencrow.db")
            db.init_schema(con)
            db.upsert_instruments(
                con,
                [
                    {
                        "symbol": "TEST",
                        "asset_type": "ETF",
                        "venue": "TSE",
                        "currency": "JPY",
                        "first_date": "2026-01-01",
                    }
                ],
            )
            item = {"symbol": "TEST", "venue": "TSE", "currency": "JPY", "source_name": "csv_market", "fixture": str(price_csv)}
            save_market_csv(con, item, tmp_path)
            build_features(con)
            close = con.execute("SELECT close_adj_jpy FROM feature_weekly WHERE week_end='2026-01-02'").fetchone()[0]
            self.assertEqual(close, 50.0)
            price_csv.write_text(price_csv.read_text(encoding="utf-8").replace("2026-01-30,108", "2026-01-30,109"), encoding="utf-8")
            save_market_csv(con, item, tmp_path)
            self.assertGreater(con.execute("SELECT COUNT(*) FROM event_log WHERE reason='price_revision'").fetchone()[0], 0)

    def test_crypto_asset_type_participates_in_feature_building(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            price_csv = tmp_path / "crypto_prices.csv"
            price_csv.write_text(
                "\n".join(
                    [
                        "date,open,high,low,close,adj_close,volume,dividend,split",
                        "2026-01-02,100,101,99,100,100,1000,0,1",
                        "2026-01-09,102,103,101,102,102,1000,0,1",
                        "2026-01-16,104,105,103,104,104,1000,0,1",
                        "2026-01-23,106,107,105,106,106,1000,0,1",
                        "2026-01-30,108,109,107,108,108,1000,0,1",
                    ]
                ),
                encoding="utf-8",
            )
            con = db.connect(tmp_path / "rencrow.db")
            db.init_schema(con)
            db.upsert_instruments(
                con,
                [
                    {
                        "symbol": "BTC-USD",
                        "asset_type": "CRYPTO",
                        "venue": "YAHOO",
                        "currency": "USD",
                        "first_date": "2026-01-01",
                    }
                ],
            )
            con.execute(
                "INSERT INTO macro_series(series_code, obs_date, value, vintage_date, release_date, source_name, fetch_id, unit) VALUES ('USDJPY_BOJ', '2026-01-01', 150, '', '2026-01-01', 'csv_macro', 1, 'JPY')"
            )
            con.commit()
            item = {"symbol": "BTC-USD", "venue": "YAHOO", "currency": "USD", "source_name": "csv_market", "fixture": str(price_csv)}
            save_market_csv(con, item, tmp_path)
            build_features(con)
            iid = con.execute("SELECT instrument_id FROM instruments WHERE symbol='BTC-USD'").fetchone()[0]
            self.assertGreater(
                con.execute("SELECT COUNT(*) FROM feature_weekly WHERE instrument_id=?", (iid,)).fetchone()[0],
                0,
            )

    def test_feature_builds_12w_skip1_momentum(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            con = db.connect(tmp_path / "rencrow.db")
            db.init_schema(con)
            db.upsert_instruments(
                con,
                [
                    {
                        "symbol": "1306.T",
                        "asset_type": "ETF",
                        "venue": "TSE",
                        "currency": "JPY",
                        "first_date": "2026-01-01",
                    }
                ],
            )
            item = {
                "symbol": "1306.T",
                "venue": "TSE",
                "currency": "JPY",
                "source_name": "csv_market",
                "fixture": str(ROOT / "fixtures" / "1306T_prices.csv"),
            }
            save_market_csv(con, item, tmp_path)
            build_features(con)
            row = con.execute(
                """
                SELECT ret_12w, ret_12w_skip1
                  FROM feature_weekly
                 WHERE week_end='2026-04-03'
                """
            ).fetchone()
            self.assertAlmostEqual(row["ret_12w"], 113 / 101 - 1)
            self.assertAlmostEqual(row["ret_12w_skip1"], 112 / 100 - 1)


if __name__ == "__main__":
    unittest.main()
