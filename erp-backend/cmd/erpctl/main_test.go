package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name, status string
		want         int
	}{
		{name: "complete", status: "complete", want: 0},
		{name: "incomplete", status: "incomplete", want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ERP_TOKEN", "test-token")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authorization")
				}
				var body any
				switch r.Method + " " + r.URL.Path {
				case "POST /api/ai/agent/sessions":
					body = map[string]any{"id": 7, "debug": true}
				case "POST /api/ai/agent/sessions/7/messages":
					var request map[string]string
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request["message"] != "销售情况" || request["thinking"] != "deep" {
						t.Errorf("request: %v", request)
					}
					body = map[string]any{"message": map[string]any{"id": 9, "session_id": 7, "status": tt.status}}
				case "GET /api/ai/agent/messages/9/trace":
					body = map[string]any{
						"message": map[string]any{
							"id": 9, "session_id": 7, "status": tt.status, "content": "销售额900",
						},
						"trace": map[string]any{"stop_reason": "done"}, "debug": true,
					}
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": body}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			var out, stderr bytes.Buffer
			args := []string{"agent", "ask", "销售情况", "--url", server.URL, "--thinking", "deep", "--json"}
			if code := run(context.Background(), args, &out, &stderr); code != tt.want {
				t.Fatalf("code=%d stderr=%s", code, &stderr)
			}
			var result diagnostic
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Message.ID != 9 || !strings.Contains(string(result.Trace), "done") {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestParseOptions(t *testing.T) {
	t.Setenv("ERP_URL", "")
	tests := []struct {
		name string
		args []string
		bad  bool
	}{
		{name: "trailing flags", args: []string{"question", "--json", "--session", "4"}},
		{name: "invalid url", args: []string{"--url", "file:///etc/passwd"}, bad: true},
		{name: "url credentials", args: []string{"--url", "https://secret@example.com"}, bad: true},
		{name: "zero timeout", args: []string{"--timeout", "0s"}, bad: true},
		{name: "missing flag value", args: []string{"--timeout"}, bad: true},
		{name: "invalid thinking", args: []string{"--thinking", "maximum"}, bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseOptions(tt.args)
			if (err != nil) != tt.bad {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestClientRequestErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		delay  time.Duration
	}{
		{name: "unauthorized", status: 401, body: `{"code":401,"msg":"expired"}`},
		{name: "application error", status: 200, body: `{"code":500,"msg":"failed"}`},
		{name: "SPA fallback", status: 200, body: `<html>index</html>`},
		{name: "timeout", status: 200, body: `{"code":200}`, delay: 40 * time.Millisecond},
		{name: "redirect", status: 302, body: `{"code":200}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(tt.delay)
				w.Header().Set("Location", "https://example.com")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			c := client{base: server.URL, http: &http.Client{
				Timeout: 20 * time.Millisecond,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}}
			if _, err := c.request(context.Background(), "GET", "/test", nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCollectLogsRejectsInjection(t *testing.T) {
	tests := []struct {
		name string
		opts options
	}{
		{name: "container", opts: options{container: "app;id", since: "1m"}},
		{name: "duration", opts: options{container: "app", since: "1m;id"}},
		{name: "host", opts: options{container: "app", since: "1m", target: "router", host: "host;id"}},
		{name: "ssh option", opts: options{container: "app", since: "1m", target: "router", host: "-oProxyCommand=id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := collectLogs(context.Background(), tt.opts); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDoctor(t *testing.T) {
	t.Setenv("ERP_CONFIG_DIR", t.TempDir()+"/credentials")
	tests := []struct {
		name, token string
		ready       bool
		want        int
	}{
		{name: "basic without token", want: 0},
		{name: "AI configured", token: "token", ready: true, want: 0},
		{name: "AI missing", token: "token", want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ERP_TOKEN", tt.token)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := map[string]any{"code": 200, "initialized": true}
				if r.URL.Path == "/api/system/ai/providers" {
					response["data"] = map[string]any{"active": "env", "env_configured": tt.ready}
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			var out, stderr bytes.Buffer
			args := []string{"doctor", "--url", server.URL, "--json"}
			if code := run(context.Background(), args, &out, &stderr); code != tt.want {
				t.Fatalf("code=%d stderr=%s", code, &stderr)
			}
			if !json.Valid(out.Bytes()) {
				t.Fatalf("invalid output %s", &out)
			}
		})
	}
}

func TestDebugModeStopsBeforeQuestionOnOldBackend(t *testing.T) {
	t.Setenv("ERP_TOKEN", "token")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/ai/agent/sessions" {
			t.Error("question sent to unsupported server")
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"id": 1}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	code := run(context.Background(), []string{"agent", "ask", "test", "--url", server.URL, "--json"}, &out, &stderr)
	if code != 1 || calls != 1 || !strings.Contains(stderr.String(), "debug mode mismatch") {
		t.Fatalf("code=%d calls=%d stderr=%s", code, calls, &stderr)
	}
}
