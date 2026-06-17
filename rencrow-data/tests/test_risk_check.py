from __future__ import annotations

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


def run_script(script: str, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(SRC / script), *args]
    return subprocess.run(cmd, cwd=REPO, text=True, capture_output=True, check=check, env={"PYTHONPATH": str(SRC)})


def write_config(tmp_path: Path) -> tuple[Path, Path, Path, Path]:
    data_root = tmp_path / "rencrow-data"
    config = data_root / "config"
    fixtures = data_root / "fixtures"
    config.mkdir(parents=True)
    fixtures.mkdir(parents=True)
    (fixtures / "prices.csv").write_text((ROOT / "fixtures" / "1306T_prices.csv").read_text(encoding="utf-8"), encoding="utf-8")
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
                        "fixture": "fixtures/prices.csv",
                    }
                ]
            }
        ),
        encoding="utf-8",
    )
    (config / "sources.yml").write_text(json.dumps({"macro_sources": []}), encoding="utf-8")
    (config / "calendars.yml").write_text(json.dumps({"calendar_sources": []}), encoding="utf-8")
    risk_config = config / "risk_limits.yml"
    risk_config.write_text(
        json.dumps(
            {
                "event_risk_stop_threshold": 0.9,
                "max_drawdown_limit": 0.99,
                "weekly_loss_limit": 0.99,
                "annualized_volatility_limit": 9.9,
                "turnover_warning_limit": 9.9,
            }
        ),
        encoding="utf-8",
    )
    return data_root, config, data_root / "data" / "rencrow.db", risk_config


def prepare_backtest(tmp_path: Path) -> tuple[Path, Path, Path, Path]:
    data_root, config_root, db_path, risk_config = write_config(tmp_path)
    run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(config_root))
    run_script("02_fetch_market.py", "--db", str(db_path), "--config-root", str(config_root), "--data-root", str(data_root))
    run_script("04_build_features.py", "--db", str(db_path))
    run_script("06_make_snapshot.py", "--db", str(db_path), "--output-dir", str(data_root / "data" / "snapshots"), "--snapshot-date", "2026-05-16")
    run_script(
        "09_backtest_weekly_rotation.py",
        "--db",
        str(db_path),
        "--snapshot",
        "latest",
        "--symbols",
        "1306.T",
        "--output-dir",
        str(data_root / "data" / "backtests"),
    )
    return data_root, config_root, db_path, risk_config


class RiskCheckTest(unittest.TestCase):
    def test_risk_check_passes_with_lenient_limits(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            _, _, db_path, risk_config = prepare_backtest(Path(td))
            result = run_script(
                "10_risk_check.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--risk-config",
                str(risk_config),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["status"], "pass")
            con = sqlite3.connect(db_path)
            row = con.execute("SELECT status FROM risk_check_result WHERE risk_check_id=?", (summary["risk_check_id"],)).fetchone()
            self.assertEqual(row[0], "pass")

    def test_risk_check_stops_on_strict_drawdown_limit(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            _, _, db_path, risk_config = prepare_backtest(tmp_path)
            risk_config.write_text(
                json.dumps(
                    {
                        "event_risk_stop_threshold": 0.9,
                        "max_drawdown_limit": 0.001,
                        "weekly_loss_limit": 0.99,
                        "annualized_volatility_limit": 9.9,
                        "turnover_warning_limit": 9.9,
                    }
                ),
                encoding="utf-8",
            )
            result = run_script(
                "10_risk_check.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--risk-config",
                str(risk_config),
                "--json",
                check=False,
            )
            self.assertEqual(result.returncode, 3)
            summary = json.loads(result.stdout)
            self.assertEqual(summary["status"], "stop")
            self.assertEqual(summary["max_dd_check"], "fail")

    def test_risk_check_reduces_on_turnover_warning(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            _, _, db_path, risk_config = prepare_backtest(Path(td))
            risk_config.write_text(
                json.dumps(
                    {
                        "event_risk_stop_threshold": 0.9,
                        "max_drawdown_limit": 0.99,
                        "weekly_loss_limit": 0.99,
                        "annualized_volatility_limit": 9.9,
                        "turnover_warning_limit": 0.0,
                    }
                ),
                encoding="utf-8",
            )
            result = run_script(
                "10_risk_check.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--risk-config",
                str(risk_config),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["status"], "reduce")
            self.assertEqual(summary["concentration_check"], "warning")

    def test_risk_check_kill_switch_on_stop_event(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            _, _, db_path, risk_config = prepare_backtest(Path(td))
            con = sqlite3.connect(db_path)
            con.execute(
                """
                INSERT INTO event_log(event_ts, scope, level, reason, event_risk_score, context_json)
                VALUES ('2026-05-15T00:00:00+00:00', 'system', 'stop', 'manual_kill_switch', 1.0, '{}')
                """
            )
            con.commit()
            con.close()
            result = run_script(
                "10_risk_check.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--risk-config",
                str(risk_config),
                "--json",
                check=False,
            )
            self.assertEqual(result.returncode, 3)
            summary = json.loads(result.stdout)
            self.assertEqual(summary["status"], "kill_switch")
            self.assertEqual(summary["event_check"], "fail")


if __name__ == "__main__":
    unittest.main()
