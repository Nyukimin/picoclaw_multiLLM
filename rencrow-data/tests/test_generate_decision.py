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


def write_config(tmp_path: Path, *, strict_risk: bool = False) -> tuple[Path, Path, Path, Path]:
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
                "event_lookback_days": 7,
                "max_drawdown_limit": 0.001 if strict_risk else 0.99,
                "weekly_loss_limit": 0.99,
                "annualized_volatility_limit": 9.9,
                "turnover_warning_limit": 9.9,
            }
        ),
        encoding="utf-8",
    )
    return data_root, config, data_root / "data" / "rencrow.db", risk_config


def prepare_decision_inputs(tmp_path: Path, *, strict_risk: bool = False) -> tuple[Path, Path]:
    data_root, config_root, db_path, risk_config = write_config(tmp_path, strict_risk=strict_risk)
    run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(config_root))
    con = sqlite3.connect(db_path)
    con.execute(
        """
        UPDATE strategy_version
           SET config_hash='unit_test_custom', config_json=?
         WHERE strategy_id='weekly_etf_rotation_v1'
        """,
        (
            json.dumps(
                {
                    "cash_proxy": "1306.T",
                    "drawdown_penalty": 0.25,
                    "score_min": -999.0,
                    "top_n": 1,
                    "universe": ["1306.T"],
                    "volatility_penalty": 0.5,
                },
                sort_keys=True,
            ),
        ),
    )
    con.commit()
    con.close()
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
    run_script(
        "10_risk_check.py",
        "--db",
        str(db_path),
        "--snapshot",
        "latest",
        "--risk-config",
        str(risk_config),
        check=not strict_risk,
    )
    return data_root, db_path


class GenerateDecisionTest(unittest.TestCase):
    def test_generate_decision_writes_signal_log_and_approval_file(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            data_root, db_path = prepare_decision_inputs(Path(td))
            result = run_script(
                "11_generate_decision.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--risk-check",
                "latest",
                "--output-dir",
                str(data_root / "approvals"),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertFalse(summary["vetoed"])
            self.assertTrue(summary["approval_required"])
            self.assertFalse(summary["approved"])
            self.assertEqual(summary["candidates"][0]["symbol"], "1306.T")
            self.assertTrue(Path(summary["approval_path"]).exists())
            self.assertTrue(summary["approval_path"].endswith(".approval.yml"))
            self.assertTrue(Path(summary["approval_latest_path"]).exists())
            self.assertTrue(Path(summary["approval_json_path"]).exists())
            self.assertEqual(Path(summary["approval_latest_path"]).name, "latest.yml")
            approval_text = Path(summary["approval_path"]).read_text(encoding="utf-8")
            self.assertIn(f"decision_id: {summary['decision_id']}", approval_text)
            self.assertIn("approval_required: true", approval_text)
            self.assertIn("approved: false", approval_text)

            con = sqlite3.connect(db_path)
            con.row_factory = sqlite3.Row
            decision = con.execute("SELECT * FROM decision_log WHERE decision_id=?", (summary["decision_id"],)).fetchone()
            self.assertIsNotNone(decision)
            self.assertEqual(decision["approved"], 0)
            self.assertIn('"approval_required":true', decision["candidate_json"])
            self.assertEqual(con.execute("SELECT COUNT(*) FROM weekly_signal").fetchone()[0], 1)

    def test_generate_decision_defaults_to_latest_risk_check(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            data_root, db_path = prepare_decision_inputs(Path(td))
            result = run_script(
                "11_generate_decision.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--output-dir",
                str(data_root / "approvals"),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["risk_status"], "pass")
            self.assertEqual(summary["candidates"][0]["symbol"], "1306.T")

    def test_generate_decision_vetoes_when_risk_stops(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            data_root, db_path = prepare_decision_inputs(Path(td), strict_risk=True)
            result = run_script(
                "11_generate_decision.py",
                "--db",
                str(db_path),
                "--snapshot",
                "latest",
                "--risk-check",
                "latest",
                "--output-dir",
                str(data_root / "approvals"),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertTrue(summary["vetoed"])
            self.assertEqual(summary["risk_status"], "stop")
            self.assertEqual(summary["candidates"][0]["target_weight"], 0.0)
            self.assertTrue(Path(summary["approval_path"]).exists())


if __name__ == "__main__":
    unittest.main()
