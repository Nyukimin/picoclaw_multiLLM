# SBV2 接続先一覧（`win11-hp01`）

現在稼働中の SBV2 External Server 接続先一覧です。

---

## `sbv2-a`

- 用途: `female_01`, `mio`
- HTTP Base URL: `http://win11-hp01:8765`
- WebSocket URL: `ws://win11-hp01:8765/sessions`
- SBV2 直呼び出し URL: `http://win11-hp01:8765/synthesis`
- Ready:

```json
{
  "status": "ready",
  "voices": ["female_01", "mio"]
}
```

---

## `sbv2-b`

- 用途: `male_01`
- HTTP Base URL: `http://win11-hp01:8766`
- WebSocket URL: `ws://win11-hp01:8766/sessions`
- SBV2 直呼び出し URL: `http://win11-hp01:8766/synthesis`
- Ready:

```json
{
  "status": "ready",
  "voices": ["male_01"]
}
```

---

## Tailscale 置換例

通常運用で Tailscale 名を使う場合は、必要に応じて `win11-hp01` を以下へ置き換える。

- `win11-hp01.tailb07d8d.ts.net`

例:

- `https://win11-hp01.tailb07d8d.ts.net:8765`
- `wss://win11-hp01.tailb07d8d.ts.net:8765/sessions`
- `https://win11-hp01.tailb07d8d.ts.net:8766`
- `wss://win11-hp01.tailb07d8d.ts.net:8766/sessions`

---

## 使い分け

- `female_01`, `mio` を使う場合は `8765`
- `male_01` を使う場合は `8766`

以上。
