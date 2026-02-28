#!/bin/bash
# MCP Chrome 接続テストスクリプト
# Win11 側で mcp-chrome-bridge が起動している必要があります

set -e

MCP_URL="http://100.83.235.65:12306"

echo "=== MCP Chrome 接続テスト ==="
echo ""

# 1. ヘルスチェック
echo "[1/3] ヘルスチェック..."
if curl -f -s "${MCP_URL}/health" > /dev/null; then
    echo "  ✓ ヘルスチェック成功"
else
    echo "  ✗ ヘルスチェック失敗"
    echo ""
    echo "Win11 側で mcp-chrome-bridge が起動しているか確認してください:"
    echo "  PowerShell> mcp-chrome-bridge --port 12306"
    exit 1
fi

# 2. ツール一覧取得
echo "[2/3] ツール一覧取得..."
TOOLS_RESPONSE=$(curl -s -X POST "${MCP_URL}/mcp" \
    -H "Content-Type: application/json" \
    -d '{"method":"tools/list","params":{}}')

if echo "$TOOLS_RESPONSE" | jq -e '.tools' > /dev/null 2>&1; then
    echo "  ✓ ツール一覧取得成功"
    echo ""
    echo "  利用可能なツール:"
    echo "$TOOLS_RESPONSE" | jq -r '.tools[] | "    - \(.name): \(.description)"'
else
    echo "  ✗ ツール一覧取得失敗"
    echo "  レスポンス: $TOOLS_RESPONSE"
    exit 1
fi

# 3. Chrome 操作テスト（example.com に移動）
echo ""
echo "[3/3] Chrome 操作テスト（example.com に移動）..."
NAVIGATE_RESPONSE=$(curl -s -X POST "${MCP_URL}/mcp" \
    -H "Content-Type: application/json" \
    -d '{"method":"tools/call","params":{"name":"chrome_navigate","arguments":{"url":"https://example.com"}}}')

if echo "$NAVIGATE_RESPONSE" | jq -e '.content' > /dev/null 2>&1; then
    echo "  ✓ Chrome 操作成功"
    echo "  結果: $(echo "$NAVIGATE_RESPONSE" | jq -r '.content[0].text')"
else
    echo "  ⚠ Chrome 操作に問題がある可能性があります"
    echo "  レスポンス: $NAVIGATE_RESPONSE"
    echo ""
    echo "  確認事項:"
    echo "  - Chrome が起動していますか？"
    echo "  - Chrome 拡張機能が有効になっていますか？"
    echo "  - Native Messaging Host が登録されていますか？"
fi

echo ""
echo "=== テスト完了 ==="
