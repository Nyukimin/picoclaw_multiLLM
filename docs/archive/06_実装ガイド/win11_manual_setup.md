# Win11 側 MCP Chrome 手動セットアップ手順

**対象マシン**: BMAX2ND (100.83.235.65)
**日付**: 2026-02-25

---

## 前提条件の確認

PowerShell を開いて以下を実行：

```powershell
# Node.js バージョン確認（v18 以上推奨）
node --version

# npm バージョン確認
npm --version

# Chrome がインストールされているか確認
Get-Process chrome -ErrorAction SilentlyContinue
```

---

## ステップ 1: pnpm のインストール

```powershell
# pnpm をグローバルインストール
npm install -g pnpm

# インストール確認
pnpm --version
```

**期待される出力**: `9.x.x` 等のバージョン番号

---

## ステップ 2: mcp-chrome-bridge のインストール確認

```powershell
# mcp-chrome-bridge バージョン確認
mcp-chrome-bridge --version

# インストールされていない場合
npm install -g mcp-chrome-bridge
```

---

## ステップ 3: Chrome 拡張機能のビルド

### オプション A: GitHub からクローン（推奨）

```powershell
# ダウンロードフォルダに移動
cd $env:USERPROFILE\Downloads

# リポジトリをクローン（まだの場合）
git clone https://github.com/hangwin/mcp-chrome.git

# ディレクトリに移動
cd mcp-chrome

# 依存関係をインストール
pnpm install

# ビルド
pnpm run build
```

**期待される出力**: `dist` フォルダが生成される

### オプション B: ビルド済みをダウンロード

1. https://github.com/hangwin/mcp-chrome/releases にアクセス
2. 最新リリースの `mcp-chrome-extension.zip` をダウンロード
3. `C:\Users\nyuki\Downloads\mcp-chrome\dist` に展開

---

## ステップ 4: Chrome 拡張機能のインストール

1. Chrome を起動
2. アドレスバーに `chrome://extensions/` と入力
3. 右上の「デベロッパーモード」を **ON** にする
4. 「パッケージ化されていない拡張機能を読み込む」ボタンをクリック
5. 以下のフォルダを選択:
   ```
   C:\Users\nyuki\Downloads\mcp-chrome\dist
   ```
6. 拡張機能が読み込まれたことを確認（アイコンが表示される）

---

## ステップ 5: Native Messaging Host の登録

```powershell
# Native Messaging Host を登録
mcp-chrome-bridge register

# 確認
mcp-chrome-bridge doctor
```

**期待される出力**:
```
✓ Native messaging host is registered
✓ Chrome extension can communicate with bridge
```

---

## ステップ 6: ファイアウォール設定

**PowerShell を管理者権限で実行**:

```powershell
# ファイアウォール規則を追加
New-NetFirewallRule -DisplayName "MCP Chrome Bridge" `
  -Direction Inbound `
  -Protocol TCP `
  -LocalPort 12306 `
  -Action Allow

# 確認
Get-NetFirewallRule -DisplayName "MCP Chrome Bridge"
```

---

## ステップ 7: mcp-chrome-bridge の起動

### 手動起動（テスト用）

```powershell
# フォアグラウンドで起動
mcp-chrome-bridge --port 12306
```

**別の PowerShell で接続テスト**:

```powershell
# ヘルスチェック
curl http://localhost:12306/health

# 期待される出力: HTTP 200 OK
```

### バックグラウンド起動（永続化）

#### オプション A: Windows サービス化（nssm 使用）

1. **nssm をダウンロード**: https://nssm.cc/download
2. **nssm.exe** を `C:\Tools\nssm\` に配置
3. PowerShell（管理者権限）で実行:

```powershell
# サービスをインストール
C:\Tools\nssm\nssm.exe install mcp-chrome-bridge `
  "C:\Program Files\nodejs\node.exe" `
  "C:\Users\nyuki\AppData\Roaming\npm\node_modules\mcp-chrome-bridge\dist\index.js" `
  "--port" "12306"

# サービスを開始
C:\Tools\nssm\nssm.exe start mcp-chrome-bridge

# サービス状態を確認
C:\Tools\nssm\nssm.exe status mcp-chrome-bridge
```

#### オプション B: タスクスケジューラで自動起動

1. タスクスケジューラを開く（`taskschd.msc`）
2. 「基本タスクの作成」を選択
3. トリガー: **コンピューターの起動時**
4. 操作: **プログラムの開始**
5. プログラム: `C:\Program Files\nodejs\node.exe`
6. 引数: `C:\Users\nyuki\AppData\Roaming\npm\node_modules\mcp-chrome-bridge\dist\index.js --port 12306`
7. 完了して保存

---

## ステップ 8: 接続テスト（Win11 ローカル）

```powershell
# ヘルスチェック
Invoke-WebRequest http://localhost:12306/health

# MCP エンドポイントテスト
$body = @{
  method = "tools/list"
  params = @{}
} | ConvertTo-Json

Invoke-WebRequest -Uri http://localhost:12306/mcp `
  -Method POST `
  -ContentType "application/json" `
  -Body $body
```

**期待される応答**:
```json
{
  "tools": [
    {"name": "chrome_navigate", "description": "..."},
    {"name": "chrome_click", "description": "..."},
    {"name": "chrome_screenshot", "description": "..."},
    {"name": "chrome_get_text", "description": "..."}
  ]
}
```

---

## ステップ 9: Tailscale 経由の接続テスト（Linux 側から）

Win11 で mcp-chrome-bridge が起動している状態で、Linux 側で実行:

```bash
# ヘルスチェック
curl http://100.83.235.65:12306/health

# 期待: HTTP 200 OK
```

---

## トラブルシューティング

### mcp-chrome-bridge が起動しない

```powershell
# ログを確認
cat ~\.mcp-chrome\mcp-chrome.log

# ポートが既に使われていないか確認
netstat -ano | findstr :12306

# プロセスを強制終了
taskkill /F /PID <プロセスID>
```

### Chrome 拡張機能が認識されない

1. Chrome で `chrome://extensions/` を開く
2. 拡張機能が有効になっているか確認
3. 拡張機能のコンソールでエラーを確認（「詳細」→「エラー」）
4. Chrome を再起動

### ファイアウォールブロック

```powershell
# Windows Defender ファイアウォールを一時的に無効化してテスト
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False

# テスト後に有効化
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled True
```

### Native Messaging Host が登録されない

```powershell
# レジストリを手動確認
Get-ItemProperty -Path "HKCU:\Software\Google\Chrome\NativeMessagingHosts\com.mcp.chrome" -ErrorAction SilentlyContinue

# 手動で修正
mcp-chrome-bridge fix-permissions
```

---

## 次のステップ

セットアップが完了したら、Linux 側で PicoClaw の統合テストを実行:

```bash
cd /home/nyukimi/picoclaw_multiLLM
go test ./pkg/mcp/... -v
```

---

**最終更新**: 2026-02-25
**作成者**: Claude Sonnet 4.5
