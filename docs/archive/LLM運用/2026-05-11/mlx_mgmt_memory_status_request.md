# MLX管理デーモン `/v1/status` メモリ項目追加依頼

以下を 8079 管理サーバー担当へ共有してください。

---

## 件名

`/v1/status` のメモリ詳細項目追加のお願い

## 背景

RenCrow Viewer の Ops タブでは、8079 管理デーモンの `GET /v1/status` をサーバー側プロキシ経由で表示しています。

現在取得できている項目:

- `memory.system.total_*`
- `memory.system.used_*`
- `memory.system.free_*`
- `memory.system.available_for_llm_*`
- `memory.system.used_for_llm_*`
- `memory.system.llm_safety_margin_*`
- `memory.system.safe_available_for_llm_*`
- `memory.llm_by_role.Chat`
- `memory.llm_by_role.Worker`

Viewer でメモリ状況を運用判断できるよう、追加で以下の項目を `/v1/status` に含めてください。

## 対象エンドポイント

- `GET /v1/status`
- `GET /mgmt/v1/status` がある場合は同内容

認証は既存どおり:

```bash
curl -sS \
  -H "Authorization: Bearer ${LLM_OPS_TOKEN}" \
  http://127.0.0.1:8079/v1/status
```

## 追加してほしい項目

`memory.system` 配下:

- `available_gib` / `available_bytes`
- `swap_used_gib` / `swap_used_bytes`
- `memory_pressure`
- `compressed_gib` / `compressed_bytes`
- `file_cache_gib` / `file_cache_bytes`
- `wired_gib` / `wired_bytes`

`memory` 配下:

- `top_memory_processes`
- `model_processes`

既存の `available_for_llm_*` / `safe_available_for_llm_*` は継続してください。`available_gib` は OS 全体の Available RAM、`available_for_llm_gib` は LLM 切替判定用の値として別扱いにします。

## 希望レスポンス形

```json
{
  "roles": {
    "Chat": {
      "health_ok": true,
      "detail": "{\"status\":\"ok\"}",
      "halted": false
    },
    "Worker": {
      "health_ok": true,
      "detail": "{\"status\":\"ok\"}",
      "halted": false
    }
  },
  "halted": [],
  "memory": {
    "system": {
      "total_bytes": 137438953472,
      "total_gib": 128.0,
      "used_bytes": 125024731136,
      "used_gib": 116.44,
      "free_bytes": 12414222336,
      "free_gib": 11.56,
      "available_bytes": 17179869184,
      "available_gib": 16.0,
      "available_for_llm_bytes": 12382740480,
      "available_for_llm_gib": 11.53,
      "used_for_llm_bytes": 125056212992,
      "used_for_llm_gib": 116.47,
      "llm_safety_margin_bytes": 8589934592,
      "llm_safety_margin_gib": 8.0,
      "safe_available_for_llm_bytes": 3792805888,
      "safe_available_for_llm_gib": 3.53,
      "swap_used_bytes": 2147483648,
      "swap_used_gib": 2.0,
      "memory_pressure": "normal",
      "compressed_bytes": 3221225472,
      "compressed_gib": 3.0,
      "file_cache_bytes": 10737418240,
      "file_cache_gib": 10.0,
      "wired_bytes": 7516192768,
      "wired_gib": 7.0
    },
    "llm_by_role": {
      "Chat": {
        "pid": 734,
        "rss_bytes": 6128795648,
        "rss_mib": 5844.88
      },
      "Worker": {
        "pid": 735,
        "rss_bytes": 19898515456,
        "rss_mib": 18976.7
      }
    },
    "top_memory_processes": [
      {
        "pid": 735,
        "name": "python",
        "command": "mlx_lm.server ... Worker",
        "rss_bytes": 19898515456,
        "rss_mib": 18976.7
      }
    ],
    "model_processes": [
      {
        "role": "Worker",
        "model": "Worker",
        "pid": 735,
        "rss_bytes": 19898515456,
        "rss_mib": 18976.7,
        "port": 8082
      },
      {
        "role": "Chat",
        "model": "Chat",
        "pid": 734,
        "rss_bytes": 6128795648,
        "rss_mib": 5844.88,
        "port": 8081
      }
    ]
  }
}
```

## macOS での取得元候補

- `vm_stat`: free / speculative / compressed / file-backed / wired など
- `sysctl hw.memsize`: total RAM
- `memory_pressure`: memory pressure 状態
- `ps -axo pid,rss,comm,args`: top memory processes / model processes
- `sysctl vm.swapusage`: swap used

単位は Viewer 側で扱いやすいよう、bytes と GiB/MiB の両方を返してください。

## 受け入れ条件

1. `GET /v1/status` の JSON に上記の追加項目が含まれる
2. 既存項目名は変更しない
3. Chat / Worker 停止時も `model_processes` または `llm_by_role` で停止状態が判別できる
4. RenCrow Viewer の Ops タブで `not reported` ではなく実値が表示される

## 補足

RenCrow Viewer はトークンをブラウザへ出さず、RenCrow サーバーが `/viewer/llm-ops/status` で 8079 をプロキシします。8079 側のレスポンスに項目が入れば、Viewer はその値を表示できます。

