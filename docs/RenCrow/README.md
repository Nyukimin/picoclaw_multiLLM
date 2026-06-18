# RenCrow Docs

RenCrow 専用 Docs は、稼働中の RenCrow が参照する横断仕様、運用仕様、CLI / Viewer の利用仕様を置く。

## 位置づけ

- 正本仕様の一次参照は `docs/01_正本仕様/` に置く。
- RenCrow 全体を横断する運用仕様、複数リポジトリにまたがる仕様、CLI / Viewer から参照する利用者向け仕様は `docs/RenCrow/` に置く。
- 個別モジュール固有の詳細仕様は、各リポジトリの `docs/` に置く。

## CLI / Viewer からの検索

稼働中 RenCrow は Docs 検索 API を提供する。

- `GET /viewer/docs/search?q=QUERY&limit=N`
- `GET /viewer/docs/detail?id=DOC_ID`

RenCrow CLI は同じ API を使う。

- `rencrow docs search QUERY`
- `rencrow docs show DOC_ID`
- `rencrow chat` の対話中に `@QUERY`

`@QUERY` は通常のチャット送信ではなく Docs 検索として扱う。通常メッセージとして `@` から始めたい場合は `@@` で始める。

## 初期検索対象

- `picoclaw_multiLLM/docs/`
- `RenCrow_CMD/docs/`
- `RenCrow_STT/docs/`
- `RenCrow_TTS/docs/`
- `RenCrow_LLM/docs/`
- `RenCrow_Vision/docs/`
- `RenCrow_Tools/docs/`

