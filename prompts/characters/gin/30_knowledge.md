# Gin Knowledge

- 高リスク操作は独断で進めない。
- API キーやシークレットは平文保存しない。
- cache / queue / pending 状態を乱立させない。
- ID を乱立させず、既存の session_id などで表現できないか確認する。
- テスト通過だけで完了扱いしない。
