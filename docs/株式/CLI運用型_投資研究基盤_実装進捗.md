# CLI運用型 投資研究基盤 実装進捗

## 位置づけ

この文書は `CLI運用型_投資研究基盤_実装仕様書.md` の実装状況を追跡するための進捗メモである。

## 実装済みCLI

| CLI | 状態 | 主な保存先 |
|---|---|---|
| `01_init_db.py` | 実装済み | schema, `strategy_version` |
| `02_fetch_market.py` | 実装済み | `price_raw`, `corporate_action`, `source_fetch_log` |
| `03_fetch_macro.py` | 実装済み | `macro_series`, `economic_calendar`, `source_fetch_log` |
| `04_build_features.py` | 実装済み | `feature_weekly` |
| `05_detect_events.py` | 実装済み | `event_log`, `feature_weekly.event_risk_score` |
| `06_make_snapshot.py` | 実装済み | `snapshot_registry`, snapshot gzip |
| `07_sync_universe.py` | 実装済み | `instruments`, config sync |
| `08_validate_data.py` | 実装済み | `data_quality_check` |
| `09_backtest_weekly_rotation.py` | 実装済み | `backtest_run`, `backtest_metric`, equity/trade CSV |
| `10_risk_check.py` | 実装済み | `risk_check_result` |
| `11_generate_decision.py` | 実装済み | `weekly_signal`, `decision_log`, approval JSON |
| `12_paper_trade.py` | 実装済み | `paper_trade_log`, `tax_lot_log` |
| `13_llm_report.py` | 実装済み | report Markdown, `llm_audit_log` |
| `14_audit_report.py` | 実装済み | audit Markdown |

## 仕様要件との対応

| 要件 | 状態 | 根拠 |
|---|---|---|
| CLIだけで週次研究フローを再現 | 実装済み | `test_weekly_cli_flow.py` |
| Make経由の週次研究フロー | 実装済み | `make rencrow-data-weekly-research` |
| systemd schedulerからの週次研究フロー | 実装済み | `scripts/rencrow_data_scheduler.sh weekly` |
| `01` から `08` までのデータ基盤CLI | 実装済み | `test_pipeline_e2e.py`, `test_quality_validation.py` |
| `feature_weekly` の再現性 | 実装済み | `test_market_features.py` |
| 直近1週skipの12週モメンタム | 実装済み | `feature_weekly.ret_12w_skip1`, `test_market_features.py` |
| snapshot hash保存 | 実装済み | `snapshot_registry`, `test_pipeline_e2e.py` |
| 週次ETF回転backtest | 実装済み | `test_backtest_weekly_rotation.py` |
| 税、手数料、スリッページ、1週ラグ | 実装済み | `backtest.py`, `test_backtest_weekly_rotation.py` |
| `train`/`test`/`oos_YYYY` split metric | 実装済み | `--walk-forward`, `test_backtest_weekly_rotation.py` |
| Calmar、平均保有期間、worst month、recovery months | 実装済み | `backtest_metric`, `test_backtest_weekly_rotation.py` |
| event vetoをbacktestへ反映 | 実装済み | `event_vetoed`, `test_backtest_weekly_rotation.py` |
| event vetoをdecisionへ反映 | 実装済み | `veto_json`, `test_generate_decision.py` |
| risk checkのpass/reduce/stop/kill | 実装済み | `test_risk_check.py` |
| decision candidate生成 | 実装済み | `test_generate_decision.py` |
| `--risk-check` 省略時の最新risk check解決 | 実装済み | `test_generate_decision.py`, `test_weekly_cli_flow.py` |
| human approval前提 | 実装済み | approval JSON, `test_paper_trade.py` |
| YAML承認ファイル対応 | 実装済み | `test_paper_trade.py` |
| paper trade | 実装済み | `paper_trade_log`, `test_paper_trade.py` |
| TCA summary | 実装済み | `paper.py`, `test_paper_trade.py` |
| 税ロット近似 | 実装済み | `tax_lot_log`, `test_paper_trade.py` |
| LLMは説明補助のみ | 実装済み | `13_llm_report.py`, `llm_audit_log` |
| LLMが売買判断を変更しない | 実装済み | report-only CLI design |
| 監査レポート | 実装済み | `14_audit_report.py`, `test_audit_report.py` |
| 紙運用完了ゲート判定 | 実装済み | `paper_gate`, `test_audit_report.py` |
| 紙運用ログ欠落チェック | 実装済み | snapshot, validation, feature, backtest, risk, paper trade, report |

## 現在の未完了・運用待ち

| 項目 | 状態 | 理由 |
|---|---|---|
| 8から12週間の紙運用継続 | 未完了 | 実期間の継続運用が必要 |
| 小額実運用仕様 | 未着手 | 紙運用完了後に別仕様化する |
| ライブ発注CLI | 対象外 | 初期MVPでは作らない |
| 証券会社API接続 | 対象外 | 初期MVPでは作らない |

## 現時点の検証コマンド

```bash
uv run --with pytest --with requests python -m pytest rencrow-data/tests
```

直近確認結果:

```text
33 passed
```

追加確認:

```bash
git diff --check
bash -n scripts/rencrow_data_scheduler.sh
make -n rencrow-data-weekly-research
make -n install-data-scheduler
```

直近確認結果:

```text
問題なし
```
