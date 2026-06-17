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


class AuditReportTest(unittest.TestCase):
    def test_audit_report_writes_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            db_path = tmp_path / "rencrow.db"
            out_dir = tmp_path / "reports"
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(tmp_path / "missing_config"))
            con = sqlite3.connect(db_path)
            con.execute(
                """
                INSERT INTO snapshot_registry(snapshot_id, snapshot_date, db_hash, features_hash, status)
                VALUES (1, '2026-05-16', 'dbhash', 'featurehash', 'success')
                """,
            )
            con.commit()
            con.close()
            result = run_script(
                "14_audit_report.py",
                "--db",
                str(db_path),
                "--snapshot",
                "1",
                "--output-dir",
                str(out_dir),
                "--json",
            )
            summary = json.loads(result.stdout)
            path = Path(summary["output_path"])
            self.assertTrue(path.exists())
            text = path.read_text(encoding="utf-8")
            self.assertIn("# RenCrow Investment Audit Report", text)
            self.assertIn("snapshot_id: 1", text)
            self.assertIn("decision: not_found", text)
            self.assertIn("Paper Operation Gate", text)
            self.assertEqual(summary["paper_gate"]["status"], "not_ready")

    def test_audit_report_marks_minimum_paper_gate_ready(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            db_path = tmp_path / "rencrow.db"
            out_dir = tmp_path / "reports"
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(tmp_path / "missing_config"))
            con = sqlite3.connect(db_path)
            for idx in range(8):
                snapshot_id = idx + 1
                decision_date = f"2026-05-{idx + 1:02d}"
                con.execute(
                    """
                    INSERT INTO snapshot_registry(snapshot_id, snapshot_date, db_hash, features_hash, status)
                    VALUES (?, ?, 'dbhash', 'featurehash', 'success')
                    """,
                    (snapshot_id, decision_date),
                )
                con.execute(
                    """
                    INSERT INTO data_quality_check(run_id, instrument_id, check_date, check_type, severity, status)
                    VALUES (?, NULL, ?, 'fetch_status', 'info', 'pass')
                    """,
                    (f"run-{idx}", decision_date),
                )
                con.execute(
                    """
                    INSERT INTO instruments(instrument_id, symbol, asset_type, venue, first_date)
                    VALUES (?, ?, 'ETF', 'TEST', '2026-01-01')
                    ON CONFLICT(symbol, venue, first_date) DO NOTHING
                    """,
                    (snapshot_id, f"T{idx}"),
                )
                con.execute(
                    """
                    INSERT INTO feature_weekly(instrument_id, week_end, close_adj_jpy)
                    VALUES (?, ?, 100)
                    """,
                    (snapshot_id, decision_date),
                )
                con.execute(
                    """
                    INSERT INTO backtest_run(backtest_id, strategy_id, snapshot_id, start_date, end_date, status)
                    VALUES (?, 'weekly_etf_rotation_v1', ?, ?, ?, 'success')
                    """,
                    (f"bt-{idx}", str(snapshot_id), decision_date, decision_date),
                )
                con.execute(
                    """
                    INSERT INTO decision_log(snapshot_id, decision_date, account_scope, strategy_name, candidate_json, veto_json, approved)
                    VALUES (?, ?, 'paper', 'weekly_etf_rotation_v1', '{}', '{}', 1)
                    """,
                    (snapshot_id, decision_date),
                )
                decision_id = con.execute("SELECT last_insert_rowid()").fetchone()[0]
                con.execute(
                    """
                    INSERT INTO risk_check_result(risk_check_id, snapshot_id, strategy_id, decision_id, status)
                    VALUES (?, ?, 'weekly_etf_rotation_v1', ?, 'pass')
                    """,
                    (f"risk-{idx}", str(snapshot_id), str(decision_id)),
                )
                con.execute(
                    """
                    INSERT INTO paper_trade_log(decision_id, instrument_id, side, quantity, decision_price, simulated_fill_price, cost_bps, status)
                    VALUES (?, NULL, 'HOLD', 0, NULL, NULL, 10, 'vetoed')
                    """,
                    (decision_id,),
                )
                con.execute(
                    """
                    INSERT INTO llm_audit_log(llm_log_id, snapshot_id, task_type, model, prompt_version, input_hash, output_hash, output_path)
                    VALUES (?, ?, 'weekly_report', 'local', 'v1', 'in', 'out', 'report.md')
                    """,
                    (f"llm-{idx}", str(snapshot_id)),
                )
            con.commit()
            con.close()
            result = run_script(
                "14_audit_report.py",
                "--db",
                str(db_path),
                "--snapshot",
                "1",
                "--output-dir",
                str(out_dir),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["paper_gate"]["status"], "minimum_ready")
            self.assertEqual(summary["paper_gate"]["paper_weeks"], 8)
            self.assertEqual(len(summary["paper_gate"]["weeks"]), 8)
            self.assertTrue(all(week["complete"] for week in summary["paper_gate"]["weeks"]))
            text = Path(summary["output_path"]).read_text(encoding="utf-8")
            self.assertIn("### Weekly Ledger", text)
            self.assertIn("| snapshot_date | decision_id | snapshot_id | complete | missing |", text)

    def test_audit_report_lists_missing_weekly_logs(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            tmp_path = Path(td)
            db_path = tmp_path / "rencrow.db"
            out_dir = tmp_path / "reports"
            run_script("01_init_db.py", "--db", str(db_path), "--config-root", str(tmp_path / "missing_config"))
            con = sqlite3.connect(db_path)
            con.execute(
                """
                INSERT INTO snapshot_registry(snapshot_id, snapshot_date, db_hash, features_hash, status)
                VALUES (1, '2026-05-16', 'dbhash', 'featurehash', 'success')
                """,
            )
            con.execute(
                """
                INSERT INTO decision_log(snapshot_id, decision_date, account_scope, strategy_name, candidate_json, veto_json, approved)
                VALUES (1, '2026-05-16', 'paper', 'weekly_etf_rotation_v1', '{}', '{}', 0)
                """,
            )
            con.commit()
            con.close()
            result = run_script(
                "14_audit_report.py",
                "--db",
                str(db_path),
                "--snapshot",
                "1",
                "--output-dir",
                str(out_dir),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["paper_gate"]["status"], "not_ready")
            self.assertEqual(summary["paper_gate"]["missing_decision_paper"], 1)
            self.assertEqual(len(summary["paper_gate"]["weeks"]), 1)
            week = summary["paper_gate"]["weeks"][0]
            self.assertFalse(week["complete"])
            self.assertIn("validation", week["missing"])
            self.assertIn("paper_trade", week["missing"])
            self.assertIn("report", week["missing"])


if __name__ == "__main__":
    unittest.main()
