package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeWebSearchRequestShape(t *testing.T) {
	raw := []byte(`{
		"model":"claude-opus-5-5",
		"messages":[{"role":"user","content":[{"type":"text","text":"Perform a web search for the query: 오늘 주요 뉴스"}]}],
		"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":8}],
		"stream":true
	}`)
	var req AnthropicRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	if !isNativeWebSearchRequest(req) {
		t.Fatalf("request not recognized: %+v", req.Tools)
	}
	if req.Tools[0].Type != "web_search_20250305" || req.Tools[0].MaxUses != 8 {
		t.Fatalf("server tool fields lost: %+v", req.Tools[0])
	}
	if got := extractNativeWebSearchQuery(req); got != "오늘 주요 뉴스" {
		t.Fatalf("query = %q", got)
	}
}

func TestInvokeKiroNativeWebSearchContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("x-amzn-kiro-profile-arn"); got != "arn:test" {
			t.Fatalf("profile = %q", got)
		}
		if got := r.Header.Get("X-Amz-Target"); got != "" {
			t.Fatalf("unexpected X-Amz-Target = %q", got)
		}

		var body struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string `json:"name"`
				Arguments struct {
					Query string `json:"query"`
				} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.JSONRPC != "2.0" || body.Method != "tools/call" || body.Params.Name != "web_search" || body.Params.Arguments.Query != "today news" {
			t.Fatalf("unexpected MCP request: %+v", body)
		}

		inner := `{"results":[{"title":"Example","url":"https://example.com","snippet":"Latest item","publishedDate":1732299319000}],"totalResults":1,"query":"today news"}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"result": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": inner}},
				"isError": false,
			},
		})
	}))
	defer srv.Close()

	token := TokenData{AccessToken: "test-token", ProfileArn: "arn:test"}
	got, err := invokeKiroNativeWebSearch(context.Background(), srv.Client(), srv.URL+"/mcp", token, "today news")
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalResults != 1 || len(got.Results) != 1 || got.Results[0].Title != "Example" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestWriteNativeWebSearchStreamContract(t *testing.T) {
	rec := httptest.NewRecorder()
	ok := writeNativeWebSearchStream(
		rec,
		"claude-opus-5-5",
		"srvtoolu_test",
		"today news",
		[]map[string]any{{"type": "web_search_result", "title": "Example", "url": "https://example.com", "encrypted_content": "Latest item"}},
		"<web_search>result</web_search>",
		10,
		20,
	)
	if !ok {
		t.Fatal("stream writer failed")
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"type":"server_tool_use"`,
		`"type":"web_search_tool_result"`,
		`"tool_use_id":"srvtoolu_test"`,
		`"type":"text_delta"`,
		`"web_search_requests":1`,
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q:\n%s", want, body)
		}
	}
}
