from __future__ import annotations

import json
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from datetime import date
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parents[0]
SRC = ROOT / "src"


def run_script(script: str, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(SRC / script), *args]
    return subprocess.run(cmd, cwd=REPO, text=True, capture_output=True, check=check, env={"PYTHONPATH": str(SRC)})


def write_config(tmp_path: Path, *, fixture_name: str = "prices.csv") -> tuple[Path, Path, Path]:
    data_root = tmp_path / "rencrow-data"
    config = data_root / "config"
    fixtures = data_root / "fixtures"
    config.mkdir(parents=True)
    fixtures.mkdir(parents=True)
    (fixtures / fixture_name).write_text((ROOT / "fixtures" / "1306T_prices.csv").read_text(encoding="utf-8"), encoding="utf-8")
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
                        "first_date": "2026-01-01",
                        "source_name": "csv_market",
                        "fixture": f"fixtures/{fixture_name}",
                    }
                ]
            }
        ),
        encoding="utf-8",
    )
    return data_root, config, data_root / "data" / "rencrow.db"


class QualityValidationTest(unittest.TestCase):
    def test_validate_data_records_pass_checks(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            data_root, config_root, db_path = write_config(tmp_path)
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(config_root))
            run_script("02_fetch_market.py", "--db", str(db_path), "--config-root", str(config_root), "--data-root", str(data_root))
            result = run_script(
                "08_validate_data.py",
                "--db",
                str(db_path),
                "--as-of",
                "2026-05-15",
                "--min-history-days",
                "140",
                "--max-missing-rate",
                "0.90",
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["blockers"], 0)
            self.assertEqual(summary["instrument_count"], 1)
            self.assertGreaterEqual(summary["total_checks"], 5)

            con = sqlite3.connect(db_path)
            statuses = {
                row[0]: row[1]
                for row in con.execute(
                    "SELECT check_type, status FROM data_quality_check WHERE run_id=?",
                    (summary["run_id"],),
                ).fetchall()
            }
            self.assertEqual(statuses["stale"], "pass")
            self.assertEqual(statuses["missing"], "pass")
            self.assertEqual(statuses["return_outlier"], "pass")
            self.assertEqual(statuses["adjustment_anomaly"], "pass")
            self.assertEqual(statuses["fetch_status"], "pass")

    def test_validate_data_exits_3_for_blockers(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            data_root, config_root, db_path = write_config(tmp_path)
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(config_root))
            run_script("02_fetch_market.py", "--db", str(db_path), "--config-root", str(config_root), "--data-root", str(data_root))
            result = run_script(
                "08_validate_data.py",
                "--db",
                str(db_path),
                "--as-of",
                "2026-06-30",
                "--min-history-days",
                "140",
                "--max-missing-rate",
                "0.90",
                "--json",
                check=False,
            )
            self.assertEqual(result.returncode, 3)
            summary = json.loads(result.stdout)
            self.assertGreater(summary["blockers"], 0)

            con = sqlite3.connect(db_path)
            stale = con.execute(
                """
                SELECT severity, status, metric_value
                  FROM data_quality_check
                 WHERE run_id=? AND check_type='stale'
                """,
                (summary["run_id"],),
            ).fetchone()
            self.assertEqual(stale[0], "blocker")
            self.assertEqual(stale[1], "fail")
            self.assertGreater(stale[2], 7)

    def test_fetch_failure_is_blocker(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            data_root, config_root, db_path = write_config(tmp_path)
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(config_root))
            run_script("02_fetch_market.py", "--db", str(db_path), "--config-root", str(config_root), "--data-root", str(data_root))
            con = sqlite3.connect(db_path)
            con.execute(
                """
                INSERT INTO source_fetch_log(source_name, endpoint, requested_at, status, error_message)
                VALUES ('csv_market', 'missing', '2026-05-14T00:00:00+00:00', 'fail', 'boom')
                """
            )
            con.commit()
            con.close()

            result = run_script(
                "08_validate_data.py",
                "--db",
                str(db_path),
                "--as-of",
                "2026-05-15",
                "--min-history-days",
                "140",
                "--max-missing-rate",
                "0.90",
                "--json",
                check=False,
            )
            self.assertEqual(result.returncode, 3)
            summary = json.loads(result.stdout)
            self.assertGreater(summary["blockers"], 0)
            con = sqlite3.connect(db_path)
            fetch_fail = con.execute(
                "SELECT severity, status FROM data_quality_check WHERE run_id=? AND check_type='fetch_fail'",
                (summary["run_id"],),
            ).fetchone()
            self.assertEqual(tuple(fetch_fail), ("blocker", "fail"))


if __name__ == "__main__":
    unittest.main()
