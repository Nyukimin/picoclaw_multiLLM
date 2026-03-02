# MCP Chrome 接続テストスクリプト（Win11 側）
# mcp-chrome-bridge が起動している状態で実行

Write-Host "=== MCP Chrome 接続テスト ===" -ForegroundColor Green
Write-Host ""

$mcpUrl = "http://localhost:12306"

# 1. ヘルスチェック
Write-Host "[1/3] ヘルスチェック..." -ForegroundColor Cyan
try {
    $healthResponse = Invoke-WebRequest -Uri "$mcpUrl/health" -Method GET -TimeoutSec 5
    if ($healthResponse.StatusCode -eq 200) {
        Write-Host "  ✓ ヘルスチェック成功" -ForegroundColor Green
    } else {
        Write-Host "  ✗ ヘルスチェック失敗 (Status: $($healthResponse.StatusCode))" -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "  ✗ ヘルスチェック失敗" -ForegroundColor Red
    Write-Host "  エラー: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "mcp-chrome-bridge が起動しているか確認してください:" -ForegroundColor Yellow
    Write-Host "  mcp-chrome-bridge --port 12306" -ForegroundColor Yellow
    exit 1
}

# 2. ツール一覧取得
Write-Host "[2/3] ツール一覧取得..." -ForegroundColor Cyan
$toolsBody = @{
    method = "tools/list"
    params = @{}
} | ConvertTo-Json

try {
    $toolsResponse = Invoke-WebRequest -Uri "$mcpUrl/mcp" `
        -Method POST `
        -ContentType "application/json" `
        -Body $toolsBody `
        -TimeoutSec 5

    $tools = ($toolsResponse.Content | ConvertFrom-Json).tools
    if ($tools) {
        Write-Host "  ✓ ツール一覧取得成功" -ForegroundColor Green
        Write-Host ""
        Write-Host "  利用可能なツール:" -ForegroundColor Cyan
        foreach ($tool in $tools) {
            Write-Host "    - $($tool.name): $($tool.description)" -ForegroundColor White
        }
    } else {
        Write-Host "  ✗ ツールが見つかりません" -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "  ✗ ツール一覧取得失敗" -ForegroundColor Red
    Write-Host "  エラー: $_" -ForegroundColor Red
    exit 1
}

# 3. Chrome 操作テスト（example.com に移動）
Write-Host ""
Write-Host "[3/3] Chrome 操作テスト（example.com に移動）..." -ForegroundColor Cyan
$navigateBody = @{
    method = "tools/call"
    params = @{
        name = "chrome_navigate"
        arguments = @{
            url = "https://example.com"
        }
    }
} | ConvertTo-Json -Depth 3

try {
    $navigateResponse = Invoke-WebRequest -Uri "$mcpUrl/mcp" `
        -Method POST `
        -ContentType "application/json" `
        -Body $navigateBody `
        -TimeoutSec 10

    $result = ($navigateResponse.Content | ConvertFrom-Json).content
    if ($result) {
        Write-Host "  ✓ Chrome 操作成功" -ForegroundColor Green
        Write-Host "  結果: $($result[0].text)" -ForegroundColor White
    } else {
        Write-Host "  ⚠ Chrome 操作に問題がある可能性があります" -ForegroundColor Yellow
    }
} catch {
    Write-Host "  ⚠ Chrome 操作に問題がある可能性があります" -ForegroundColor Yellow
    Write-Host "  エラー: $_" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  確認事項:" -ForegroundColor Cyan
    Write-Host "  - Chrome が起動していますか？" -ForegroundColor White
    Write-Host "  - Chrome 拡張機能が有効になっていますか？" -ForegroundColor White
    Write-Host "  - Native Messaging Host が登録されていますか？" -ForegroundColor White
    Write-Host ""
    Write-Host "  診断コマンド:" -ForegroundColor Cyan
    Write-Host "    mcp-chrome-bridge doctor" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=== テスト完了 ===" -ForegroundColor Green
Write-Host ""
Write-Host "次のステップ:" -ForegroundColor Cyan
Write-Host "  Linux 側から接続テスト:" -ForegroundColor White
Write-Host "    curl http://100.83.235.65:12306/health" -ForegroundColor Yellow
