# STT問い合わせ（証拠付き）

以下をそのまま送ってください。

---

件名: `/stt-ws` で `session_id` 通知未受信の確認依頼（証拠添付）

`/stt-ws` 経路の client/server 突合で、クライアントログの `session_id` が `(unknown)` のままです。  
こちらの実装では `session_info`（`session_id`）を送信しているため、Fujitsu側の実行系で同等出力が有効か確認をお願いします。

## 依頼事項
1. `/stt-ws` を処理している実プロセスのログで、`session_info` または `[stt]` の `ws_session_open` が出ているか確認  
2. 出ていない場合、実行中の `voice-bridge` が最新実装か確認  
3. 可能なら `[stt] {"event":"ws_session_open","session_id":"..."}` と `ws_final_emit` が同一ログで取れる状態にして再採取

## こちらの証拠（送信している事実）
### 実行ログ抜粋1
- 取得元: `/root/.cursor/projects/mnt-d-RenCrow/terminals/326479.txt`
- ハッシュ: `6f3ef1e0b8a1d4c24b9ab22258c5b6df5dffadcb21a21d2d4de9c6696927de16`

```text
[stt] {"ts":"2026-04-13T08:24:55.886Z","event":"ws_session_open","session_id":"sess-mnwxg0gd-w2xphi"}
[stt] {"ts":"2026-04-13T08:24:55.896Z","event":"ws_session_open","session_id":"sess-mnwxg0gn-22gxyn"}
[stt] {"ts":"2026-04-13T08:24:56.464Z","event":"ws_final_emit","session_id":"sess-mnwxg0gd-w2xphi","source":"provider","text":"A-E-U-A-O-A-O おはようございます。 提子。","text_len":28}
```

### 実行ログ抜粋2
- 取得元: `/root/.cursor/projects/mnt-d-RenCrow/terminals/913123.txt`
- ハッシュ: `0172e39d43d6a637283b01d203691fd1795583aad9a9f42a3912679d7e15e5c5`

```text
[stt] {"ts":"2026-04-13T08:23:59.683Z","event":"ws_session_open","session_id":"sess-mnwxet36-equlzn"}
[stt] {"ts":"2026-04-13T08:24:00.304Z","event":"ws_final_emit","session_id":"sess-mnwxet36-equlzn","source":"provider","text":"A-E-U-A-O-A-O おはようございます。 提子。","text_len":28}
```

## 補足
- client 側は `draft/final` を受信済みのため、STT自体は動作しています。
- ただし受領した Fujitsu 側サーバーログは `ConnState` 行のみで、`session_id` 付き本文イベントが無く突合できません。

---
