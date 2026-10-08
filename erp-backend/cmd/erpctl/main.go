// erpctl provides HTTP-based ERP diagnostics without database initialization.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var errCheck = errors.New("checks did not pass")

type options struct {
	url, token, target, host, container, since, cases, thinking, asOf, report, compare string
	timeout                                                                            time.Duration
	json, debug, passwordStdin                                                         bool
	session                                                                            uint64
	tail                                                                               int
	caseLimit                                                                          int
	caseAfterID                                                                        uint64
	caseState                                                                          string
}

type client struct {
	base, token string
	http        *http.Client
}
type envelope struct {
	Code        int             `json:"code"`
	Msg         string          `json:"msg"`
	Data        json.RawMessage `json:"data"`
	Initialized bool            `json:"initialized"`
}
type message struct {
	ID        uint64 `json:"id"`
	SessionID uint64 `json:"session_id"`
	RunID     string `json:"run_id"`
	Content   string `json:"content"`
	Status    string `json:"status"`
}
type diagnostic struct {
	Message  message         `json:"message"`
	Trace    json.RawMessage `json:"trace"`
	Task     json.RawMessage `json:"task"`
	Evidence json.RawMessage `json:"evidence"`
	Debug    bool            `json:"debug"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, out, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(out, `erpctl doctor | logs | auth login/status/logout
  agent ask <question> | agent trace <message-id> | agent cases | agent eval --cases <file> | agent backtest | agent predictions
Options: --url --target local|router --json --timeout --session --thinking
         --host --container --since --tail --case-state --case-limit --debug=true --password-stdin
Credentials: auth login or ERP_TOKEN environment variable
Remote SSH uses your existing SSH configuration`)
		return 0
	}
	command := args[0]
	rest := args[1:]
	if command == "agent" || command == "auth" {
		if len(rest) == 0 {
			fmt.Fprintln(stderr, "missing subcommand")
			return 2
		}
		command += " " + rest[0]
		rest = rest[1:]
	}
	o, positional, err := parseOptions(rest)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	valid := command == "doctor" || command == "logs" || command == "agent eval" || command == "agent cases" || command == "agent backtest" || command == "agent predictions" ||
		command == "auth login" || command == "auth status" || command == "auth logout"
	needsArg := command == "agent ask" || command == "agent trace"
	if (!valid && !needsArg) || (needsArg && len(positional) != 1) || (valid && len(positional) != 0) {
		fmt.Fprintln(stderr, "unknown command or invalid arguments; use erpctl help")
		return 2
	}
	if command == "agent eval" && o.cases == "" {
		fmt.Fprintln(stderr, "--cases is required")
		return 2
	}
	if command != "logs" && o.target == "router" && o.url == "http://localhost:8080" {
		fmt.Fprintln(stderr, "router target requires --url or ERP_URL")
		return 2
	}
	c := client{base: strings.TrimRight(o.url, "/"), token: o.token, http: &http.Client{
		Timeout:       o.timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
	if command != "logs" && !strings.HasPrefix(command, "auth ") {
		if err := c.restoreCredential(); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	var result any
	switch command {
	case "auth login":
		c.token = ""
		var password string
		password, err = readLoginPassword(ctx, o, stderr)
		if err == nil {
			result, err = c.login(ctx, password)
		}
	case "auth status":
		result, err = c.authStatus(ctx)
	case "auth logout":
		err = deleteCredential(c.base)
		result = map[string]any{"url": canonicalURL(c.base), "local_credentials_removed": err == nil,
			"environment_token_present": os.Getenv("ERP_TOKEN") != ""}
	case "doctor":
		result, err = c.doctor(ctx)
	case "logs":
		result, err = collectLogs(ctx, o)
	case "agent ask":
		var d diagnostic
		d, err = c.ask(ctx, positional[0], o)
		result = d
		if err == nil && d.Message.Status != "complete" {
			err = errCheck
		}
	case "agent trace":
		var id uint64
		id, err = strconv.ParseUint(positional[0], 10, 64)
		if err != nil || id == 0 {
			fmt.Fprintln(stderr, "invalid message id")
			return 2
		}
		result, err = c.trace(ctx, id)
	case "agent eval":
		result, err = c.evaluate(ctx, o)
	case "agent cases":
		result, err = c.agentCases(ctx, o)
	case "agent backtest":
		var response envelope
		response, err = c.request(ctx, "POST", "/api/ai/agent/predictions/evaluate", map[string]any{})
		if err == nil {
			result = response.Data
		}
	case "agent predictions":
		var response envelope
		response, err = c.request(ctx, "GET", "/api/ai/agent/predictions", nil)
		if err == nil {
			result = response.Data
		}
	}
	if result != nil {
		if e := printResult(out, result, o.json); e != nil {
			fmt.Fprintln(stderr, e)
			return 1
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, errCheck) {
			return 3
		}
		return 1
	}
	return 0
}

func parseOptions(args []string) (options, []string, error) {
	o := options{token: os.Getenv("ERP_TOKEN")}
	f := flag.NewFlagSet("erpctl", flag.ContinueOnError)
	f.StringVar(&o.url, "url", envDefault("ERP_URL", "http://localhost:8080"), "ERP server URL")
	f.StringVar(&o.target, "target", "local", "local or router")
	f.StringVar(&o.host, "host", os.Getenv("ERP_SSH_HOST"), "SSH host or configured alias")
	f.StringVar(&o.container, "container", "gubuy-erp-app", "Docker container")
	f.StringVar(&o.since, "since", "10m", "log duration")
	f.StringVar(&o.cases, "cases", "", "evaluation JSON file")
	f.StringVar(&o.asOf, "as-of", "", "fixed reference date for debug evaluation (YYYY-MM-DD)")
	f.StringVar(&o.report, "report", "", "write evaluation JSON report to this file")
	f.StringVar(&o.compare, "compare", "", "compare pass/fail totals with a previous evaluation report")
	f.StringVar(&o.thinking, "thinking", "standard", "quick, standard, deep, auto")
	f.DurationVar(&o.timeout, "timeout", 5*time.Minute, "per-request timeout")
	f.BoolVar(&o.json, "json", false, "JSON output")
	f.BoolVar(&o.debug, "debug", true, "use isolated debug sessions; --debug=false for normal sessions")
	f.BoolVar(&o.passwordStdin, "password-stdin", false, "read login password from stdin")
	f.Uint64Var(&o.session, "session", 0, "reuse session")
	f.IntVar(&o.tail, "tail", 200, "log line limit")
	f.IntVar(&o.caseLimit, "case-limit", 100, "agent case result limit (1-100)")
	f.Uint64Var(&o.caseAfterID, "case-after-id", 0, "return agent cases with a larger ID")
	f.StringVar(&o.caseState, "case-state", "", "agent case state filter; empty for all")
	// Standard flag stops at the first positional; normalize so trailing flags work.
	flags := []string{}
	positional := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		booleanFlag := name == "json" || name == "debug" || name == "password-stdin"
		helpFlag := name == "help" || name == "h"
		if strings.Contains(arg, "=") || booleanFlag || helpFlag {
			continue
		}
		if i+1 >= len(args) {
			return o, positional, fmt.Errorf("missing value for %s", arg)
		}
		i++
		flags = append(flags, args[i])
	}
	if err := f.Parse(flags); err != nil {
		return o, positional, err
	}
	u, err := url.Parse(o.url)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return o, positional, errors.New("invalid ERP URL")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return o, positional, errors.New("URL must not have query or fragment")
	}
	if o.timeout <= 0 || o.tail < 1 || o.tail > 10000 || o.caseLimit < 1 || o.caseLimit > 100 {
		return o, positional, errors.New("invalid timeout or tail")
	}
	if o.target != "local" && o.target != "router" {
		return o, positional, errors.New("invalid target")
	}
	switch o.thinking {
	case "quick", "standard", "deep", "auto":
	default:
		return o, positional, errors.New("invalid thinking level")
	}
	o.url = canonicalURL(o.url)
	return o, positional, nil
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (c client) request(ctx context.Context, method, path string, body any) (envelope, error) {
	var response envelope
	payload := []byte{}
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return response, fmt.Errorf("encode request: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return response, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return response, errors.New("request failed (network, cancellation or timeout); check ERP URL and service")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return response, fmt.Errorf("read response: %w", err)
	}
	if len(raw) > 8<<20 {
		return response, errors.New("response exceeds 8 MiB")
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, fmt.Errorf("HTTP %d: expected ERP JSON response", resp.StatusCode)
	}
	if resp.StatusCode != 200 || response.Code != 200 {
		if resp.StatusCode == http.StatusUnauthorized || response.Code == http.StatusUnauthorized {
			return response, errors.New("authentication rejected or expired; run erpctl auth login " +
				"(unset ERP_TOKEN if it overrides saved login)")
		}
		return response, fmt.Errorf("HTTP %d / ERP %d: %s", resp.StatusCode, response.Code, response.Msg)
	}
	return response, nil
}

func (c client) trace(ctx context.Context, id uint64) (diagnostic, error) {
	var d diagnostic
	r, err := c.request(ctx, "GET", fmt.Sprintf("/api/ai/agent/messages/%d/trace", id), nil)
	if err != nil {
		return d, fmt.Errorf("get trace (requires updated backend): %w", err)
	}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return d, fmt.Errorf("decode trace: %w", err)
	}
	if d.Message.ID != id {
		return d, errors.New("trace returned unexpected message id")
	}
	return d, nil
}

func (c client) ask(ctx context.Context, question string, o options) (diagnostic, error) {
	var d diagnostic
	if c.token == "" {
		return d, errors.New("run erpctl auth login or set ERP_TOKEN")
	}
	if strings.TrimSpace(question) == "" {
		return d, errors.New("question is empty")
	}
	id, err := c.prepareSession(ctx, o)
	if err != nil {
		return d, err
	}

	body := map[string]string{"message": question, "thinking": o.thinking}
	if o.asOf != "" {
		body["reference_date"] = o.asOf
	}
	r, err := c.request(ctx, "POST", fmt.Sprintf("/api/ai/agent/sessions/%d/messages", id), body)
	if err != nil {
		return d, fmt.Errorf("ask session %d (request may already be saved; inspect session before retrying): %w", id, err)
	}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return d, fmt.Errorf("decode answer: %w", err)
	}
	if d.Message.ID == 0 {
		return d, errors.New("server returned empty message id")
	}
	trace, err := c.trace(ctx, d.Message.ID)
	if err != nil {
		return d, err
	}
	if trace.Message.SessionID != id || trace.Debug != o.debug {
		return d, errors.New("trace does not match session or debug mode")
	}
	return trace, nil
}

func (c client) doctor(ctx context.Context) (any, error) {
	checks := []map[string]any{}
	status, err := c.request(ctx, "GET", "/api/admin/status", nil)
	checks = append(checks, map[string]any{"check": "service_database", "ok": err == nil, "detail": errorText(err)})
	if err == nil {
		checks = append(checks, map[string]any{"check": "admin_initialized", "ok": status.Initialized})
	}
	if c.token == "" {
		checks = append(checks, map[string]any{
			"check": "admin_ai_config", "status": "skipped",
			"detail": "set ERP_TOKEN for authenticated checks",
		})
	} else {
		response, e := c.request(ctx, "GET", "/api/system/ai/providers", nil)
		checks = append(checks, map[string]any{"check": "admin_ai_config", "ok": e == nil, "detail": errorText(e)})
		if e != nil {
			err = e
		} else {
			var settings struct {
				Active        string `json:"active"`
				EnvConfigured bool   `json:"env_configured"`
				Providers     []struct {
					Provider  string `json:"provider"`
					Model     string `json:"model"`
					KeyUsable bool   `json:"key_usable"`
				} `json:"providers"`
			}
			if e := json.Unmarshal(response.Data, &settings); e != nil {
				checks = append(checks, map[string]any{"check": "ai_ready", "ok": false, "detail": "invalid provider settings"})
				err = e
			} else {
				ready := settings.Active == "env" && settings.EnvConfigured
				for _, provider := range settings.Providers {
					if provider.Provider == settings.Active {
						ready = provider.KeyUsable && provider.Model != ""
					}
				}
				checks = append(checks, map[string]any{"check": "ai_ready", "ok": ready, "active": settings.Active})
				if !ready {
					err = errCheck
				}
			}
		}
	}
	result := map[string]any{"checks": checks}
	if err != nil || !status.Initialized {
		return result, errCheck
	}
	return result, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func printResult(out io.Writer, result any, asJSON bool) error {
	if !asJSON {
		if d, ok := result.(diagnostic); ok {
			_, err := fmt.Fprintf(out, "session_id=%d message_id=%d status=%s\n%s\n",
				d.Message.SessionID, d.Message.ID, d.Message.Status, d.Message.Content)
			if err != nil {
				return fmt.Errorf("write answer: %w", err)
			}
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.@:-]*$`)

func collectLogs(ctx context.Context, o options) (any, error) {
	duration, err := time.ParseDuration(o.since)
	if err != nil || duration <= 0 {
		return nil, errors.New("--since must be a positive duration, such as 10m")
	}
	if !safeName.MatchString(o.container) {
		return nil, errors.New("invalid container name")
	}
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	args := []string{
		"logs", "--timestamps", "--since",
		strconv.FormatFloat(duration.Seconds(), 'f', -1, 64) + "s",
		"--tail", strconv.Itoa(o.tail), o.container,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if o.target == "router" {
		if !safeName.MatchString(o.host) {
			return nil, errors.New("--host or ERP_SSH_HOST is required and must be an SSH host/alias")
		}
		cmd = exec.CommandContext(
			ctx,
			"ssh",
			"-o", "BatchMode=yes", "-o", "ConnectTimeout=5",
			o.host, "docker "+strings.Join(args, " "),
		)
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("collect docker logs: %w", err)
	}
	return map[string]any{"target": o.target, "container": o.container, "logs": string(raw)}, nil
}

// prepareSession requires server acknowledgement before a debug question is sent.
func (c client) prepareSession(ctx context.Context, o options) (uint64, error) {
	var session struct {
		ID    uint64 `json:"id"`
		Debug bool   `json:"debug"`
	}
	if o.session == 0 {
		r, err := c.request(ctx, "POST", "/api/ai/agent/sessions", map[string]bool{"debug": o.debug})
		if err != nil {
			return 0, err
		}
		if err := json.Unmarshal(r.Data, &session); err != nil {
			return 0, fmt.Errorf("decode session: %w", err)
		}
	} else {
		r, err := c.request(ctx, "GET", fmt.Sprintf("/api/ai/agent/sessions/%d", o.session), nil)
		if err != nil {
			return 0, err
		}
		var data struct {
			Session json.RawMessage `json:"session"`
		}
		if err := json.Unmarshal(r.Data, &data); err != nil {
			return 0, fmt.Errorf("decode session response: %w", err)
		}
		if err := json.Unmarshal(data.Session, &session); err != nil {
			return 0, fmt.Errorf("decode session: %w", err)
		}
	}
	if session.ID == 0 {
		return 0, errors.New("server returned empty session id")
	}
	if o.session != 0 && session.ID != o.session {
		return 0, errors.New("server returned unexpected session id")
	}
	if session.Debug != o.debug {
		return 0, fmt.Errorf(
			"session %d debug mode mismatch; update backend or select a matching session (normal mode: --debug=false)",
			session.ID,
		)
	}
	return session.ID, nil
}
