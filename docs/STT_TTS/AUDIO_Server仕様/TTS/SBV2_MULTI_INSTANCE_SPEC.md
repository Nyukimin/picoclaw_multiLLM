# SBV2 複数インスタンス運用仕様

このドキュメントは、`services/sbv2_external_server` を **2本以上**動かす場合の運用仕様です。  
対象は、**同一宅内ルータ配下の複数 SBV2 インスタンス**を、**通常は Tailscale 経由**で利用する構成です。

---

## 1. 結論

**音声を2本動かす場合は、原則として「1プロセスに voice を2つ載せる」よりも、「SBV2 External Server を 2 インスタンス起動する」構成を推奨**します。

理由:

- 音声生成の遅延と失敗をインスタンス単位で分離できる
- モデル切替やキャッシュを分離できる
- 将来 3本以上に増やすときに拡張しやすい
- `voice_id` 単位ではなく **インスタンス単位で ready / health / restart** を扱える

---

## 2. 推奨構成

### 2.1 基本形

2本動かす場合は、少なくとも次の 2 インスタンスを用意する。

- `sbv2-a`
- `sbv2-b`

各インスタンスは以下を**分離**する:

- リッスンポート
- `cache_dir`
- `voice_registry.yml`
- `public_base_url`
- 必要に応じて `model_root`

共有してよいもの:

- `RENCROW_SBV2_ROOT`（SBV2 本体）
- 同一マシン上の Python 実行環境
- 同一宅内ルータ LAN

### 2.2 ネットワーク形

- **通常運用**: クライアントからの到達は **Tailscale**
- **緊急時**: **ユーザーエンド以外のみ** 宅内 IP を利用可

例:

- `https://sbv2-a.<tailnet>.ts.net:8765`
- `https://sbv2-b.<tailnet>.ts.net:8766`

または同一ホスト名でポート分離:

- `https://audio.<tailnet>.ts.net:8765`
- `https://audio.<tailnet>.ts.net:8766`

---

## 3. 単一インスタンス多voiceとの違い

### 3.1 単一インスタンス多voice

1プロセスの `voice_registry.yml` に複数 voice を載せる方式。

向いているケース:

- 切替対象が軽い
- 同時発話数が少ない
- 障害分離を強く求めない

弱点:

- 1つのプロセス障害で全 voice が落ちる
- キャッシュとログが混ざりやすい
- モデル切替やロード競合の影響を受けやすい

### 3.2 複数インスタンス

voice または用途ごとにサーバを分ける方式。

向いているケース:

- 2本以上を安定運用したい
- 応答遅延を分離したい
- モデルや voice の責務を分けたい
- 将来的に GPU / PC / ポートを増やしたい

**今回はこちらを標準構成とする。**

---

## 4. インスタンス分離ルール

## 4.1 必須分離項目

各インスタンスで以下を必ず分離する。

| 項目 | 分離要否 | 理由 |
|------|----------|------|
| `RENCROW_SBV2_PORT` | 必須 | ポート衝突回避 |
| `RENCROW_SBV2_CACHE_DIR` | 必須 | `audio_path` / 一時生成物の衝突回避 |
| `RENCROW_SBV2_VOICE_REGISTRY` | 必須 | 役割と voice の分離 |
| `RENCROW_SBV2_PUBLIC_BASE_URL` | 必須 | `audio_url` を正しく返すため |
| `RENCROW_SBV2_DEFAULT_VOICE_ID` | 推奨 | インスタンスごとの既定 voice 固定 |
| `RENCROW_SBV2_MODEL_ROOT` | 条件付き | モデル群を完全分離したい場合 |
| ログ出力先 | 推奨 | 障害切り分け容易化 |

## 4.2 共有可能項目

| 項目 | 共有可否 | 備考 |
|------|----------|------|
| `RENCROW_SBV2_ROOT` | 可 | SBV2 本体コード |
| Python 実行環境 | 可 | ただし依存は同一に保つ |
| 宅内ルータ配下 LAN | 可 | 通常運用は Tailscale 優先 |

---

## 5. ルーティング仕様

## 5.1 固定割当

最初は **固定割当** を推奨する。

例:

- `voice_id=female_01` は `sbv2-a`
- `voice_id=male_01` は `sbv2-b`

この場合、上位クライアントまたはブリッジ層で `voice_id` に応じて `base_url` / `ws_url` を選ぶ。

## 5.2 セッション固定

WebSocket セッションは、**`session_start` を受けたインスタンスに最後まで固定**する。

理由:

- `seq` 管理がインスタンス内状態に依存する
- `chunk_index` を跨いで別インスタンスへ移すと順序保証が壊れる
- `audio_path` / `audio_url` の生成先が変わる

ルール:

- 1つの `session_id` は 1つのインスタンスにのみ所属
- `session_end` までは同一インスタンスで完結

## 5.3 負荷分散

現時点では **透過ロードバランサでのラウンドロビンは非推奨**。

理由:

- `/sessions` はステートフル
- `/synthesis` と `/synthesize` でも `audio_path` の所在がインスタンス依存

もし負荷分散するなら、次のどちらかにする。

- **voice 固定ルーティング**
- **session affinity を持つディスパッチャ**

---

## 6. Ready / Live の扱い

各インスタンスは独立して `GET /health/live` と `GET /health/ready` を持つ。

期待仕様:

- `sbv2-a` が down でも `sbv2-b` は ready のままでよい
- 監視はインスタンス単位
- `voices` はそのインスタンスが担当するものだけ返す

例:

### `sbv2-a`

```json
{
  "status": "ready",
  "voices": ["female_01", "mio"]
}
```

### `sbv2-b`

```json
{
  "status": "ready",
  "voices": ["male_01"]
}
```

---

## 7. voice_registry 仕様

複数本運用では、**1つの巨大な registry を全インスタンスで共有するよりも、インスタンスごとに registry を分ける**。

### `voice_registry.a.yml`

```yaml
default_voice_id: female_01
voices:
  female_01:
    model_name: amitaro
    speaker_id: 0
    style: Neutral
    style_weight: 2.0
    language: JP
  mio:
    alias_of: female_01
```

### `voice_registry.b.yml`

```yaml
default_voice_id: male_01
voices:
  male_01:
    model_name: shin-gozaki-jp
    speaker_id: 0
    style: Neutral
    style_weight: 2.0
    language: JP
```

---

## 8. 環境変数仕様

## 8.1 `sbv2-a`

```bash
RENCROW_SBV2_PORT=8765
RENCROW_SBV2_CACHE_DIR=/mnt/d/RenCrow/services/sbv2_external_server/cache-a
RENCROW_SBV2_VOICE_REGISTRY=/mnt/d/RenCrow/services/sbv2_external_server/voice_registry.a.yml
RENCROW_SBV2_PUBLIC_BASE_URL=https://sbv2-a.<tailnet>.ts.net:8765
RENCROW_SBV2_DEFAULT_VOICE_ID=female_01
RENCROW_SBV2_CONCURRENCY=1
```

## 8.2 `sbv2-b`

```bash
RENCROW_SBV2_PORT=8766
RENCROW_SBV2_CACHE_DIR=/mnt/d/RenCrow/services/sbv2_external_server/cache-b
RENCROW_SBV2_VOICE_REGISTRY=/mnt/d/RenCrow/services/sbv2_external_server/voice_registry.b.yml
RENCROW_SBV2_PUBLIC_BASE_URL=https://sbv2-b.<tailnet>.ts.net:8766
RENCROW_SBV2_DEFAULT_VOICE_ID=male_01
RENCROW_SBV2_CONCURRENCY=1
```

## 8.3 宅内 IP フォールバック

Tailscale 不調時に限り、**ユーザーエンド以外**の内部設定では以下のように切り替えてよい。

```bash
RENCROW_SBV2_PUBLIC_BASE_URL=http://192.168.1.10:8765
```

ただし、これは**緊急用**であり、通常運用の既定にしてはならない。

---

## 9. 同時実行数

複数本動かす場合でも、**各インスタンスの `RENCROW_SBV2_CONCURRENCY` は最初は `1` を推奨**する。

推奨理由:

- SBV2 はモデルロードと推論が重い
- 同一 GPU / CPU 上での同時推論は遅延悪化を招きやすい
- インスタンスを増やす目的が「安定分離」であるため

最初の推奨:

- `sbv2-a`: `CONCURRENCY=1`
- `sbv2-b`: `CONCURRENCY=1`

その後、実測で問題なければ個別に増やす。

---

## 10. 起動仕様

起動順は次を推奨する。

1. `sbv2-a`
2. `sbv2-b`
3. 必要なら上位クライアント / ブリッジ

各インスタンスの起動後に確認する項目:

1. `/health/live`
2. `/health/ready`
3. `/synthesis` で 1 回の音声生成
4. `/audio/...` で音声取得

---

## 11. 監視・ログ

複数本運用では、ログに **instance_id** を必ず持たせる。

最低限必要な識別子:

- `instance_id`
- `request_id`
- `session_id`
- `voice_id`
- `elapsed_ms`
- `result`

推奨:

- `instance_id=sbv2-a`
- `instance_id=sbv2-b`

---

## 12. 障害時の扱い

### 12.1 片系障害

- `sbv2-a` が down でも `sbv2-b` は継続稼働
- 片系にしかない `voice_id` は利用不可
- 共有 voice を持たせていない限り、自動フェイルオーバはしない

### 12.2 フェイルオーバ

自動フェイルオーバは**任意機能**とする。初期実装では不要。

理由:

- `voice_id` とモデル構成を一致させる必要がある
- `audio_path` / `audio_url` / キャッシュ所在が変わる
- WS セッション途中の引き継ぎは難しい

---

## 13. 受け入れ条件

2本運用の完了条件は以下。

- [ ] `sbv2-a` と `sbv2-b` が別ポートで同時起動する
- [ ] 両方の `/health/ready` が独立して `ready` を返す
- [ ] `voice_id` に応じて期待したインスタンスへルーティングできる
- [ ] 各インスタンスの `audio_path` / `audio_url` が衝突しない
- [ ] `sbv2-a` 停止時も `sbv2-b` は継続利用できる
- [ ] Tailscale 不調時、ユーザーエンド以外は宅内 IP に切替可能

---

## 14. 実運用の推奨方針

**「2本動かす」時の標準方針は次のとおり。**

1. SBV2 External Server を **2プロセス**起動する
2. **ポート / cache / registry / public_base_url を分離**する
3. voice は **固定割当** する
4. WebSocket セッションは **インスタンス固定** にする
5. 通常は **Tailscale**
6. 緊急時のみ **ユーザーエンド以外で宅内 IP** を使う

以上。
