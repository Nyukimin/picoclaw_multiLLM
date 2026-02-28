package mcp

import (
	"context"
	"fmt"
)

// ChromeNavigate はブラウザで指定 URL に移動
func (c *Client) ChromeNavigate(ctx context.Context, url string) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_navigate", map[string]interface{}{
		"url": url,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	result, ok := resp.Content[0]["text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}

// ChromeClick は指定セレクタの要素をクリック
func (c *Client) ChromeClick(ctx context.Context, selector string) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_click", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	result, ok := resp.Content[0]["text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}

// ChromeScreenshot はページのスクリーンショットを取得
func (c *Client) ChromeScreenshot(ctx context.Context) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_screenshot", map[string]interface{}{})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	// Base64 エンコードされた画像データを返す
	result, ok := resp.Content[0]["data"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}

// ChromeGetText は指定セレクタの要素のテキストを取得
func (c *Client) ChromeGetText(ctx context.Context, selector string) (string, error) {
	resp, err := c.CallTool(ctx, "chrome_get_text", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}

	result, ok := resp.Content[0]["text"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	return result, nil
}
