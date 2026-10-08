package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testToken(t *testing.T, expiry time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{"username": "test_admin", "exp": expiry.Unix()}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-only-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuthLoginStatusLogout(t *testing.T) {
	t.Setenv("ERP_TOKEN", "")
	dir := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("ERP_CONFIG_DIR", dir)
	token := testToken(t, time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/verify":
			if r.Header.Get("Authorization") != "" {
				t.Error("login should not use saved token")
			}
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if input["password"] != "test-password" {
				t.Error("wrong password payload")
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{
				"token": token, "username": "test_admin",
			}}); err != nil {
				t.Error(err)
			}
		case "/api/system/ai/providers":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("saved token not used")
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"code": 200}); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("test-password\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = original
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	var out, stderr bytes.Buffer
	args := []string{"auth", "login", "--password-stdin", "--url", server.URL, "--json"}
	if code := run(context.Background(), args, &out, &stderr); code != 0 {
		t.Fatalf("code=%d error=%s", code, &stderr)
	}
	output := out.String() + stderr.String()
	if strings.Contains(output, token) || strings.Contains(output, "test-password") {
		t.Fatal("secret leaked in output")
	}
	saved, err := loadCredential(server.URL)
	if err != nil || saved.Token != token {
		t.Fatalf("credential load failed: %v", err)
	}
	for path, mode := range map[string]os.FileMode{dir: 0700, filepath.Join(dir, credentialName(server.URL)): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("permissions invalid for %s: %v", path, err)
		}
	}
	out.Reset()
	stderr.Reset()
	statusArgs := []string{"auth", "status", "--url", server.URL, "--json"}
	if code := run(context.Background(), statusArgs, &out, &stderr); code != 0 {
		t.Fatalf("status failed: %d %s", code, &stderr)
	}
	var state authState
	if err := json.Unmarshal(out.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Authenticated || state.Source != "saved" {
		t.Fatalf("state=%+v", state)
	}
	out.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"auth", "logout", "--url", server.URL}, &out, &stderr); code != 0 {
		t.Fatalf("logout failed: %d %s", code, &stderr)
	}
	saved, err = loadCredential(server.URL)
	if err != nil || saved.Token != "" {
		t.Fatal("logout did not remove credential")
	}
}

func TestCredentialIsolationAndExpiry(t *testing.T) {
	t.Setenv("ERP_CONFIG_DIR", filepath.Join(t.TempDir(), "credentials"))
	expired := testToken(t, time.Now().Add(-time.Hour))
	if err := saveCredential(credential{URL: "https://EXAMPLE.com:443/erp/", Token: expired}); err != nil {
		t.Fatal(err)
	}
	c := client{base: "https://example.com/erp"}
	if err := c.restoreCredential(); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expiry error: %v", err)
	}
	other, err := loadCredential("https://example.com/another")
	if err != nil || other.Token != "" {
		t.Fatal("credential crossed URL boundary")
	}
	c.token = "environment-override"
	if err := c.restoreCredential(); err != nil || c.token != "environment-override" {
		t.Fatal("environment token not preferred")
	}
}

func TestCredentialRejectsUnsafeFiles(t *testing.T) {
	tests := []struct {
		name               string
		symlink, directory bool
	}{
		{name: "world readable file"},
		{name: "symlink", symlink: true},
		{name: "world accessible directory", directory: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "credentials")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ERP_CONFIG_DIR", dir)
			path := filepath.Join(dir, credentialName("http://localhost:8080"))
			if err := os.WriteFile(path, []byte(`{"url":"http://localhost:8080","token":"secret"}`), 0600); err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.directory:
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case tt.symlink:
				actual := filepath.Join(dir, "actual")
				if err := os.Rename(path, actual); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(actual, path); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := loadCredential("http://localhost:8080"); err == nil {
				t.Fatal("unsafe credential accepted")
			}
		})
	}
}

func TestAuthStatusRequiresServerVerification(t *testing.T) {
	t.Setenv("ERP_CONFIG_DIR", filepath.Join(t.TempDir(), "credentials"))
	token := testToken(t, time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("token not sent for verification")
		}
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]any{"code": 401}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	c := client{base: server.URL, token: token, http: server.Client()}
	result, err := c.authStatus(context.Background())
	state := result.(authState)
	if err == nil || state.Authenticated {
		t.Fatal("unverified token reported as authenticated")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatal("token leaked")
	}
}

func TestFailedLoginPreservesSavedCredential(t *testing.T) {
	t.Setenv("ERP_CONFIG_DIR", filepath.Join(t.TempDir(), "credentials"))
	token := testToken(t, time.Now().Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]any{"code": 401}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	if err := saveCredential(credential{URL: server.URL, Token: token}); err != nil {
		t.Fatal(err)
	}
	c := client{base: server.URL, http: server.Client()}
	if _, err := c.login(context.Background(), "wrong-password"); err == nil {
		t.Fatal("failed login accepted")
	}
	saved, err := loadCredential(server.URL)
	if err != nil || saved.Token != token {
		t.Fatal("failed login destroyed saved credentials")
	}
}
