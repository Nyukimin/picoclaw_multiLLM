from __future__ import annotations

import gzip
import json
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parents[0]
SRC = ROOT / "src"


def run_script(script: str, *args: str) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(SRC / script), *args]
    return subprocess.run(cmd, cwd=REPO, text=True, capture_output=True, check=True, env={"PYTHONPATH": str(SRC)})


def write_fixture_tree(tmp_path: Path) -> tuple[Path, Path, Path]:
    data_root = tmp_path / "rencrow-data"
    config = data_root / "config"
    fixtures = data_root / "fixtures"
    config.mkdir(parents=True)
    fixtures.mkdir(parents=True)
    (fixtures / "prices.csv").write_text((ROOT / "fixtures" / "1306T_prices.csv").read_text(encoding="utf-8"), encoding="utf-8")
    (fixtures / "macro.csv").write_text((ROOT / "fixtures" / "macro_series.csv").read_text(encoding="utf-8"), encoding="utf-8")
    (fixtures / "calendar.csv").write_text((ROOT / "fixtures" / "economic_calendar.csv").read_text(encoding="utf-8"), encoding="utf-8")
    (config / "instruments.yml").write_text(
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
                        "source_name": "csv_market",
                        "fixture": "fixtures/prices.csv",
                    },
                    {
                        "symbol": "USDJPY_BOJ",
                        "name": "BOJ USDJPY",
                        "asset_type": "FX",
                        "venue": "BOJ",
                        "currency": "JPY",
                        "timezone": "Asia/Tokyo",
                        "active": 1,
                        "first_date": "2000-01-01",
                    },
                    {
                        "symbol": "DGS10",
                        "name": "US 10Y",
                        "asset_type": "RATE",
                        "venue": "FRED",
                        "currency": "USD",
                        "timezone": "UTC",
                        "active": 1,
                        "first_date": "2000-01-01",
                    },
                ]
            }
        ),
        encoding="utf-8",
    )
    (config / "sources.yml").write_text(json.dumps({"macro_sources": [{"source_name": "csv_macro", "fixture": "fixtures/macro.csv"}]}), encoding="utf-8")
    (config / "calendars.yml").write_text(json.dumps({"calendar_sources": [{"source_name": "csv_calendar", "fixture": "fixtures/calendar.csv"}]}), encoding="utf-8")
    return data_root, config, data_root / "data" / "rencrow.db"


class PipelineE2ETest(unittest.TestCase):
    def test_offline_pipeline_e2e_creates_features_events_and_snapshot(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            data_root, config_root, db_path = write_fixture_tree(tmp_path)
            run_script("01_init_db.py", "--db-path", str(db_path), "--config-root", str(config_root))
            run_script("02_fetch_market.py", "--db", str(db_path), "--config-root", str(config_root), "--data-root", str(data_root))
            run_script("03_fetch_macro.py", "--db", str(db_path), "--config-root", str(config_root), "--data-root", str(data_root))
            run_script("04_build_features.py", "--db-path", str(db_path), "--week-end", "latest")
            run_script("05_detect_events.py", "--db-path", str(db_path), "--week-end", "latest")
            run_script(
                "08_validate_data.py",
                "--db",
                str(db_path),
                "--as-of",
                "2026-05-15",
                "--min-history-days",
                "140",
                "--max-missing-rate",
                "0.90",
            )
            out_dir = data_root / "data" / "snapshots"
            run_script("06_make_snapshot.py", "--db-path", str(db_path), "--output-dir", str(out_dir), "--snapshot-date", "2026-05-16")

            con = sqlite3.connect(db_path)
            con.row_factory = sqlite3.Row
            self.assertEqual(con.execute("SELECT COUNT(*) FROM instruments").fetchone()[0], 3)
            self.assertEqual(con.execute("SELECT COUNT(*) FROM price_raw").fetchone()[0], 20)
            self.assertGreaterEqual(con.execute("SELECT COUNT(*) FROM macro_series").fetchone()[0], 20)
            self.assertGreater(con.execute("SELECT COUNT(*) FROM data_quality_check").fetchone()[0], 0)
            self.assertGreater(con.execute("SELECT COUNT(*) FROM feature_weekly WHERE ret_12w IS NOT NULL").fetchone()[0], 0)
            self.assertGreater(con.execute("SELECT COUNT(*) FROM event_log WHERE reason LIKE 'calendar_%'").fetchone()[0], 0)
            self.assertEqual(con.execute("SELECT MAX(event_risk_score) FROM feature_weekly").fetchone()[0], 0.7)
            snap = con.execute("SELECT * FROM snapshot_registry WHERE snapshot_date='2026-05-16'").fetchone()
            self.assertIsNotNone(snap)
            self.assertEqual(snap["status"], "success")
            self.assertTrue(snap["db_hash"])
            self.assertTrue(snap["features_hash"])
            snapshot_path = Path(snap["snapshot_path"])
            self.assertTrue(snapshot_path.exists())
            with gzip.open(snapshot_path, "rb") as f:
                self.assertTrue(f.read(16).startswith(b"SQLite format 3"))


    def test_snapshot_is_blocked_when_fetch_failed(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            data_root, config_root, db_path = write_fixture_tree(tmp_path)
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(config_root))
            con = sqlite3.connect(db_path)
            con.execute(
                "INSERT INTO source_fetch_log(source_name, endpoint, requested_at, status, error_message) VALUES ('csv_market', 'missing', '2026-01-01T00:00:00Z', 'fail', 'boom')"
            )
            con.commit()
            con.close()
            run_script("06_make_snapshot.py", "--db", str(db_path), "--output-dir", str(data_root / "data" / "snapshots"), "--snapshot-date", "2026-05-16")
            con = sqlite3.connect(db_path)
            status = con.execute("SELECT status FROM snapshot_registry WHERE snapshot_date='2026-05-16'").fetchone()[0]
            self.assertEqual(status, "blocked")


if __name__ == "__main__":
    unittest.main()
