# RenCrow Data Foundation

This directory contains the stock/ETF learning data foundation described in
`docs/株式/学習基盤_実装仕様書.md`.

MVP commands:

```bash
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/01_init_db.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/02_fetch_market.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/03_fetch_macro.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/04_build_features.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/05_detect_events.py
PYTHONPATH=rencrow-data/src python3 rencrow-data/src/06_make_snapshot.py
```

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
