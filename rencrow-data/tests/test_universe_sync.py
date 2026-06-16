from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"


class UniverseSyncTest(unittest.TestCase):
    def test_broad_universe_preset_merges_and_fetches(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            data_root = tmp_path / "rencrow-data"
            config_root = data_root / "config"
            fixtures = data_root / "fixtures"
            config_root.mkdir(parents=True)
            fixtures.mkdir(parents=True)

            (fixtures / "prices.csv").write_text(
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

            (config_root / "instruments.yml").write_text(
                json.dumps(
                    {
                        "instruments": [
                            {
                                "symbol": "1306.T",
                                "name": "TOPIX ETF",
                                "asset_type": "ETF",
                                "venue": "TSE",
                                "currency": "JPY",
                                "timezone": "Asia/Tokyo",
                                "active": 1,
                                "first_date": "2001-01-01",
                                "provider": "yahoo",
                                "provider_symbol": "1306.T",
                                "source_name": "csv_market",
                                "fixture": "fixtures/prices.csv",
                            }
                        ]
                    }
                ),
                encoding="utf-8",
            )

            db_path = data_root / "data" / "rencrow.db"
            cmd = [
                sys.executable,
                str(SRC / "07_sync_universe.py"),
                "--db",
                str(db_path),
                "--config-root",
                str(config_root),
                "--data-root",
                str(data_root),
                "--mode",
                "fixture",
            ]
            result = subprocess.run(cmd, cwd=ROOT.parents[1], text=True, capture_output=True, check=True, env={"PYTHONPATH": str(SRC)})
            self.assertIn("universe_sync rounds=", result.stdout)
            config = json.loads((config_root / "instruments.yml").read_text(encoding="utf-8"))
            symbols = {item["symbol"] for item in config["instruments"]}
            self.assertIn("2558.T", symbols)
            self.assertIn("BTC-USD", symbols)
            self.assertGreaterEqual(len(symbols), 20)


if __name__ == "__main__":
    unittest.main()
