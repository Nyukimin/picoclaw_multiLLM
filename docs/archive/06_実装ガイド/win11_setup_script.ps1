# Win11 側 MCP Chrome セットアップスクリプト
# 実行方法: PowerShell を管理者権限で開いて実行

Write-Host "=== MCP Chrome セットアップ開始 ===" -ForegroundColor Green

# 1. 前提条件の確認
Write-Host "`n[1/7] 前提条件の確認..." -ForegroundColor Cyan
$nodeVersion = node --version 2>$null
if ($nodeVersion) {
    Write-Host "  ✓ Node.js: $nodeVersion" -ForegroundColor Green
} else {
    Write-Host "  ✗ Node.js が見つかりません。https://nodejs.org/ からインストールしてください。" -ForegroundColor Red
    exit 1
}

$npmVersion = npm --version 2>$null
if ($npmVersion) {
    Write-Host "  ✓ npm: $npmVersion" -ForegroundColor Green
} else {
    Write-Host "  ✗ npm が見つかりません。" -ForegroundColor Red
    exit 1
}

# Chrome のパスを確認
$chromePaths = @(
    "C:\Program Files\Google\Chrome\Application\chrome.exe",
    "C:\Program Files (x86)\Google\Chrome\Application\chrome.exe",
    "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe"
)
$chromeFound = $false
foreach ($path in $chromePaths) {
    if (Test-Path $path) {
        Write-Host "  ✓ Chrome: $path" -ForegroundColor Green
        $chromeFound = $true
        break
    }
}
if (-not $chromeFound) {
    Write-Host "  ✗ Chrome が見つかりません。インストールしてください。" -ForegroundColor Red
    exit 1
}

# 2. pnpm のインストール
Write-Host "`n[2/7] pnpm のインストール..." -ForegroundColor Cyan
$pnpmVersion = pnpm --version 2>$null
if ($pnpmVersion) {
    Write-Host "  ✓ pnpm はすでにインストールされています: $pnpmVersion" -ForegroundColor Green
} else {
    Write-Host "  pnpm をインストール中..." -ForegroundColor Yellow
    npm install -g pnpm
    if ($LASTEXITCODE -eq 0) {
        $pnpmVersion = pnpm --version
        Write-Host "  ✓ pnpm インストール完了: $pnpmVersion" -ForegroundColor Green
    } else {
        Write-Host "  ✗ pnpm のインストールに失敗しました" -ForegroundColor Red
        exit 1
    }
}

# 3. mcp-chrome-bridge のインストール確認
Write-Host "`n[3/7] mcp-chrome-bridge の確認..." -ForegroundColor Cyan
$bridgeVersion = mcp-chrome-bridge --version 2>$null
if ($bridgeVersion) {
    Write-Host "  ✓ mcp-chrome-bridge: $bridgeVersion" -ForegroundColor Green
} else {
    Write-Host "  mcp-chrome-bridge をインストール中..." -ForegroundColor Yellow
    npm install -g mcp-chrome-bridge
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  ✓ mcp-chrome-bridge インストール完了" -ForegroundColor Green
    } else {
        Write-Host "  ✗ mcp-chrome-bridge のインストールに失敗しました" -ForegroundColor Red
        exit 1
    }
}

# 4. Chrome 拡張機能のビルド
Write-Host "`n[4/7] Chrome 拡張機能のビルド..." -ForegroundColor Cyan
$mcpChromeDir = "$env:USERPROFILE\Downloads\mcp-chrome"

if (-not (Test-Path $mcpChromeDir)) {
    Write-Host "  mcp-chrome リポジトリをクローン中..." -ForegroundColor Yellow
    Set-Location "$env:USERPROFILE\Downloads"
    git clone https://github.com/hangwin/mcp-chrome.git
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  ✗ リポジトリのクローンに失敗しました" -ForegroundColor Red
        Write-Host "  手動で https://github.com/hangwin/mcp-chrome/releases からダウンロードしてください" -ForegroundColor Yellow
        exit 1
    }
}

Set-Location $mcpChromeDir
Write-Host "  依存関係をインストール中..." -ForegroundColor Yellow
pnpm install
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ✗ 依存関係のインストールに失敗しました" -ForegroundColor Red
    exit 1
}

Write-Host "  拡張機能をビルド中..." -ForegroundColor Yellow
pnpm run build
if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ ビルド完了: $mcpChromeDir\dist" -ForegroundColor Green
} else {
    Write-Host "  ✗ ビルドに失敗しました" -ForegroundColor Red
    exit 1
}

# 5. Native Messaging Host の登録
Write-Host "`n[5/7] Native Messaging Host の登録..." -ForegroundColor Cyan
mcp-chrome-bridge register
if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Native Messaging Host 登録完了" -ForegroundColor Green
} else {
    Write-Host "  ✗ 登録に失敗しました" -ForegroundColor Red
    exit 1
}

# 6. ファイアウォール設定
Write-Host "`n[6/7] ファイアウォール設定..." -ForegroundColor Cyan
$firewallRule = Get-NetFirewallRule -DisplayName "MCP Chrome Bridge" -ErrorAction SilentlyContinue
if ($firewallRule) {
    Write-Host "  ✓ ファイアウォール規則はすでに存在します" -ForegroundColor Green
} else {
    Write-Host "  ファイアウォール規則を追加中..." -ForegroundColor Yellow
    New-NetFirewallRule -DisplayName "MCP Chrome Bridge" `
        -Direction Inbound `
        -Protocol TCP `
        -LocalPort 12306 `
        -Action Allow `
        -ErrorAction SilentlyContinue
    if ($?) {
        Write-Host "  ✓ ファイアウォール規則追加完了" -ForegroundColor Green
    } else {
        Write-Host "  ⚠ ファイアウォール規則の追加に失敗しました（管理者権限が必要）" -ForegroundColor Yellow
    }
}

# 7. 診断実行
Write-Host "`n[7/7] 診断実行..." -ForegroundColor Cyan
mcp-chrome-bridge doctor
if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ 診断完了" -ForegroundColor Green
} else {
    Write-Host "  ⚠ 診断で問題が検出されました" -ForegroundColor Yellow
}

# 完了メッセージ
Write-Host "`n=== セットアップ完了 ===" -ForegroundColor Green
Write-Host ""
Write-Host "次のステップ:" -ForegroundColor Cyan
Write-Host "1. Chrome で chrome://extensions/ を開く"
Write-Host "2. 「デベロッパーモード」を ON にする"
Write-Host "3. 「パッケージ化されていない拡張機能を読み込む」をクリック"
Write-Host "4. 以下のフォルダを選択: $mcpChromeDir\dist"
Write-Host ""
Write-Host "5. mcp-chrome-bridge を起動:" -ForegroundColor Cyan
Write-Host "   mcp-chrome-bridge --port 12306" -ForegroundColor Yellow
Write-Host ""
Write-Host "6. 別の PowerShell で接続テスト:" -ForegroundColor Cyan
Write-Host "   curl http://localhost:12306/health" -ForegroundColor Yellow
