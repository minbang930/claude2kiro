package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/sgeraldes/claude2kiro/internal/config"
	"github.com/sgeraldes/claude2kiro/internal/tui/logger"
)

const nativeWebSearchQueryPrefix = "Perform a web search for the query:"

type kiroWebSearchResponse struct {
	Results      []kiroWebSearchResult `json:"results"`
	TotalResults int                   `json:"totalResults"`
	Query        string                `json:"query"`
}

type kiroWebSearchResult struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Snippet       string `json:"snippet"`
	PublishedDate *int64 `json:"publishedDate"`
}

type kiroMCPResponse struct {
	Result *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func isNativeWebSearchRequest(req AnthropicRequest) bool {
	if len(req.Tools) != 1 {
		return false
	}
	tool := req.Tools[0]
	return tool.Name == "web_search" && strings.HasPrefix(tool.Type, "web_search_")
}

func extractNativeWebSearchQuery(req AnthropicRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role != "user" {
			continue
		}
		text := strings.TrimSpace(getMessageContent(msg.Content))
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, nativeWebSearchQueryPrefix) {
			text = strings.TrimSpace(strings.TrimPrefix(text, nativeWebSearchQueryPrefix))
		}
		return text
	}
	return ""
}

func nativeWebSearchProfileArn(token TokenData) string {
	if token.ProfileArn != "" {
		return token.ProfileArn
	}
	if token.AuthMethod != "IdC" {
		return consumerProfileArn
	}
	return ""
}

func nativeWebSearchEndpoint(token TokenData) string {
	region := strings.TrimSpace(config.Get().Advanced.AWSRegion)
	if region == "" {
		region = strings.TrimSpace(token.Region)
	}
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("https://q.%s.amazonaws.com/mcp", region)
}

func invokeKiroNativeWebSearch(ctx context.Context, client *http.Client, endpoint string, token TokenData, query string) (*kiroWebSearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("web search query is empty")
	}

	body, err := json.Marshal(map[string]any{
		"id":      "web_search_tooluse_" + strings.ReplaceAll(generateUUID(), "-", ""),
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "web_search",
			"arguments": map[string]string{"query": query},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal Kiro MCP request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Kiro MCP request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	ua := fmt.Sprintf("KiroIDE-%s-%s", kiroVersion, runtime.GOOS)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("x-amz-user-agent", ua)
	req.Header.Set("x-amzn-codewhisperer-optout", "false")
	req.Header.Set("amz-sdk-invocation-id", generateUUID())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")
	if profileArn := nativeWebSearchProfileArn(token); profileArn != "" {
		req.Header.Set("x-amzn-kiro-profile-arn", profileArn)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Kiro MCP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read Kiro MCP response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Kiro MCP status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var rpc kiroMCPResponse
	if err := json.Unmarshal(respBody, &rpc); err != nil {
		return nil, fmt.Errorf("parse Kiro MCP response: %w", err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("Kiro MCP error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if rpc.Result == nil || rpc.Result.IsError {
		return nil, fmt.Errorf("Kiro MCP web_search returned an error")
	}

	for _, item := range rpc.Result.Content {
		if item.Type != "text" || item.Text == "" {
			continue
		}
		var out kiroWebSearchResponse
		if err := json.Unmarshal([]byte(item.Text), &out); err != nil {
			return nil, fmt.Errorf("parse embedded Kiro web search results: %w", err)
		}
		return &out, nil
	}
	return nil, fmt.Errorf("Kiro MCP web_search returned no text result")
}

func nativeWebSearchResultBlocks(results []kiroWebSearchResult) []map[string]any {
	blocks := make([]map[string]any, 0, len(results))
	for _, result := range results {
		block := map[string]any{
			"type":              "web_search_result",
			"title":             result.Title,
			"url":               result.URL,
			"encrypted_content": result.Snippet,
		}
		if result.PublishedDate != nil && *result.PublishedDate > 0 {
			block["page_age"] = time.UnixMilli(*result.PublishedDate).UTC().Format("January 2, 2006")
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func nativeWebSearchSummary(query string, results []kiroWebSearchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<web_search>\nSearch results for %q:\n\n", query)
	if len(results) == 0 {
		b.WriteString("No results found.\n")
	}
	for i, result := range results {
		fmt.Fprintf(&b, "%d. %s\n", i+1, result.Title)
		if result.URL != "" {
			fmt.Fprintf(&b, "   URL: %s\n", result.URL)
		}
		if result.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", result.Snippet)
		}
		b.WriteByte('\n')
	}
	b.WriteString("</web_search>")
	return b.String()
}

func handleNativeWebSearchRequest(ctx context.Context, w http.ResponseWriter, req AnthropicRequest, token TokenData, lg *logger.Logger, sessionID, requestID string) (int, string) {
	query := extractNativeWebSearchQuery(req)
	if query == "" {
		writeNativeWebSearchError(w, http.StatusBadRequest, "cannot extract web search query")
		return http.StatusBadRequest, ""
	}

	endpoint := nativeWebSearchEndpoint(token)
	client := &http.Client{Timeout: config.Get().Network.HTTPTimeout}
	result, err := invokeKiroNativeWebSearch(ctx, client, endpoint, token, query)
	if err != nil {
		if lg != nil {
			lg.LogError(fmt.Sprintf("Native WebSearch failed [%s:%s]: %v", sessionID, requestID, err))
		}
		writeNativeWebSearchError(w, http.StatusBadGateway, "Kiro native web search failed")
		return http.StatusBadGateway, ""
	}

	if lg != nil {
		lg.LogInfo(fmt.Sprintf("Native WebSearch [%s:%s] results=%d", sessionID, requestID, len(result.Results)))
	}

	toolUseID := "srvtoolu_" + strings.ReplaceAll(generateUUID(), "-", "")
	resultBlocks := nativeWebSearchResultBlocks(result.Results)
	summary := nativeWebSearchSummary(query, result.Results)
	inputTokens := len(query) / 4
	if inputTokens < 1 {
		inputTokens = 1
	}
	outputTokens := len(summary) / 4
	if outputTokens < 1 {
		outputTokens = 1
	}

	if req.Stream {
		if !writeNativeWebSearchStream(w, req.Model, toolUseID, query, resultBlocks, summary, inputTokens, outputTokens) {
			return http.StatusInternalServerError, ""
		}
		return http.StatusOK, summary
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":   "msg_" + strings.ReplaceAll(generateUUID(), "-", ""),
		"type": "message",
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "server_tool_use", "id": toolUseID, "name": "web_search", "input": map[string]string{"query": query}},
			map[string]any{"type": "web_search_tool_result", "tool_use_id": toolUseID, "content": resultBlocks},
			map[string]any{"type": "text", "text": summary},
		},
		"model":         req.Model,
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"server_tool_use": map[string]any{
				"web_search_requests": 1,
			},
		},
	})
	return http.StatusOK, summary
}

func writeNativeWebSearchStream(w http.ResponseWriter, model, toolUseID, query string, results []map[string]any, summary string, inputTokens, outputTokens int) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return false
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	messageID := "msg_" + strings.ReplaceAll(generateUUID(), "-", "")
	sendSSEEvent(w, flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	}, nil)

	sendSSEEvent(w, flusher, "content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]any{
			"type": "server_tool_use", "id": toolUseID, "name": "web_search", "input": map[string]any{},
		},
	}, nil)
	queryJSON, _ := json.Marshal(map[string]string{"query": query})
	sendSSEEvent(w, flusher, "content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": string(queryJSON)},
	}, nil)
	sendSSEEvent(w, flusher, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}, nil)

	sendSSEEvent(w, flusher, "content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": 1,
		"content_block": map[string]any{
			"type": "web_search_tool_result", "tool_use_id": toolUseID, "content": results,
		},
	}, nil)
	sendSSEEvent(w, flusher, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 1}, nil)

	sendSSEEvent(w, flusher, "content_block_start", map[string]any{
		"type": "content_block_start", "index": 2,
		"content_block": map[string]any{"type": "text", "text": ""},
	}, nil)
	sendSSEEvent(w, flusher, "content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 2,
		"delta": map[string]any{"type": "text_delta", "text": summary},
	}, nil)
	sendSSEEvent(w, flusher, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 2}, nil)

	sendSSEEvent(w, flusher, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"server_tool_use": map[string]any{"web_search_requests": 1},
		},
	}, nil)
	sendSSEEvent(w, flusher, "message_stop", map[string]any{"type": "message_stop"}, nil)
	return true
}

func writeNativeWebSearchError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{"type": "api_error", "message": message},
	})
}
