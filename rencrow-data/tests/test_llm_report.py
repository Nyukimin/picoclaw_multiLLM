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


class LLMReportTest(unittest.TestCase):
    def test_llm_report_writes_output_and_audit_log(self) -> None:
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
                """
            )
            con.commit()
            con.close()
            result = run_script(
                "13_llm_report.py",
                "--db",
                str(db_path),
                "--snapshot",
                "1",
                "--output-dir",
                str(out_dir),
                "--json",
            )
            summary = json.loads(result.stdout)
            self.assertEqual(summary["model"], "local-deterministic")
            self.assertTrue(Path(summary["output_path"]).exists())
            con = sqlite3.connect(db_path)
            row = con.execute("SELECT task_type, output_path FROM llm_audit_log WHERE llm_log_id=?", (summary["llm_log_id"],)).fetchone()
            self.assertEqual(row[0], "weekly_report")
            self.assertEqual(row[1], summary["output_path"])


if __name__ == "__main__":
    unittest.main()
