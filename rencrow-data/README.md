# RenCrow Data Foundation

This directory contains the stock/ETF/crypto learning data foundation described in
`docs/株式/学習基盤_実装仕様書.md`.

MVP commands:

```bash
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/01_init_db.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/02_fetch_market.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/03_fetch_macro.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/04_build_features.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/05_detect_events.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/06_make_snapshot.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/08_validate_data.py --as-of latest
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/09_backtest_weekly_rotation.py --snapshot latest
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/10_risk_check.py --snapshot latest
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/11_generate_decision.py --snapshot latest
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/13_llm_report.py --snapshot latest --decision latest
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/14_audit_report.py --snapshot latest --decision latest
```

The same weekly research flow can be run through Make:

```bash
make rencrow-data-weekly-research
```

Paper trading requires an explicit approval file and does not place broker orders:

```bash
make rencrow-data-paper-trade DATA_APPROVAL_FILE=rencrow-data/approvals/latest.yml
```

Generated databases, snapshots, backtest CSVs, approval files, and reports are
runtime artifacts. Keep them out of git unless a specific fixture is being added.

For historical backfill from online providers:

```bash
make rencrow-data-backfill
```

For persistent scheduled runs:

```bash
make install-data-scheduler enable-data-scheduler
make data-scheduler-status
```

The default config points to local fixture files. Real provider adapters should
keep secrets in `.env` or GitHub Actions Secrets, never in config files.
