package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type credential struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type authState struct {
	URL           string     `json:"url"`
	Username      string     `json:"username,omitempty"`
	Source        string     `json:"source"`
	Authenticated bool       `json:"authenticated"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

func canonicalURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
		host := u.Hostname()
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		u.Host = host
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

func credentialDir() (string, error) {
	if dir := os.Getenv("ERP_CONFIG_DIR"); dir != "" {
		return filepath.Abs(dir)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate credential directory: %w", err)
	}
	return filepath.Join(dir, "erpctl"), nil
}

func credentialName(base string) string {
	sum := sha256.Sum256([]byte(canonicalURL(base)))
	return hex.EncodeToString(sum[:]) + ".json"
}

func secureDirectory(create bool) (string, error) {
	dir, err := credentialDir()
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", fmt.Errorf("create credential directory: %w", err)
		}
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("credential directory must be a real directory")
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("credential directory permissions must be 0700")
	}
	return dir, nil
}

func loadCredential(base string) (credential, error) {
	var saved credential
	dir, err := secureDirectory(false)
	if errors.Is(err, os.ErrNotExist) {
		return saved, nil
	}
	if err != nil {
		return saved, err
	}
	path := filepath.Join(dir, credentialName(base))
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return saved, nil
	}
	if err != nil {
		return saved, fmt.Errorf("inspect credential: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return saved, errors.New("credential must be a regular file with 0600 permissions")
	}
	if info.Size() > 64<<10 {
		return saved, errors.New("credential file too large")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return saved, fmt.Errorf("read credential: %w", err)
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		return saved, errors.New("credential is invalid; run auth login again")
	}
	if saved.URL != canonicalURL(base) || saved.Token == "" {
		return saved, errors.New("credential does not match server URL")
	}
	return saved, nil
}

func saveCredential(saved credential) (err error) {
	dir, err := secureDirectory(true)
	if err != nil {
		return err
	}
	saved.URL = canonicalURL(saved.URL)
	target := filepath.Join(dir, credentialName(saved.URL))
	info, inspectErr := os.Lstat(target)
	if inspectErr != nil && !errors.Is(inspectErr, os.ErrNotExist) {
		return inspectErr
	}
	if inspectErr == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return errors.New("existing credential must be a regular file with 0600 permissions")
	}
	raw, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}
	file, err := os.CreateTemp(dir, ".credential-*")
	if err != nil {
		return fmt.Errorf("create credential file: %w", err)
	}
	name := file.Name()
	defer func() {
		if cleanupErr := os.Remove(name); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary credential: %w", cleanupErr))
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close credential file: %w", err)
	}
	if err := os.Rename(name, target); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}
	return nil
}

func deleteCredential(base string) error {
	dir, err := secureDirectory(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, credentialName(base)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	return nil
}

// tokenMetadata is an expiry hint only. The server verifies the signature.
func tokenMetadata(token string) (string, *time.Time, error) {
	var claims struct {
		Username string `json:"username"`
		jwt.RegisteredClaims
	}
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil {
		return "", nil, errors.New("invalid token; run auth login")
	}
	if claims.ExpiresAt == nil {
		return "", nil, errors.New("token has no expiry; run auth login")
	}
	expiration := claims.ExpiresAt.Time
	return claims.Username, &expiration, nil
}

func (c *client) restoreCredential() error {
	if c.token != "" {
		return nil
	}
	saved, err := loadCredential(c.base)
	if err != nil {
		return err
	}
	if saved.Token == "" {
		return nil
	}
	_, expiration, err := tokenMetadata(saved.Token)
	if err != nil {
		return err
	}
	if !expiration.After(time.Now()) {
		return errors.New("saved token expired; run erpctl auth login")
	}
	c.token = saved.Token
	return nil
}

func (c client) login(ctx context.Context, password string) (any, error) {
	response, err := c.request(ctx, "POST", "/api/admin/verify", map[string]string{"password": password})
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	var data struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, errors.New("invalid login response")
	}
	username, expiration, err := tokenMetadata(data.Token)
	if err != nil {
		return nil, err
	}
	if !expiration.After(time.Now()) || username != data.Username {
		return nil, errors.New("login returned invalid credentials")
	}
	saved := credential{URL: c.base, Username: data.Username, Token: data.Token}
	if err := saveCredential(saved); err != nil {
		return nil, err
	}
	return authState{
		URL: canonicalURL(c.base), Username: username, Source: "saved",
		Authenticated: true, ExpiresAt: expiration,
	}, nil
}

func (c client) authStatus(ctx context.Context) (any, error) {
	state := authState{URL: canonicalURL(c.base), Source: "none"}
	var err error
	if c.token != "" {
		state.Source = "environment"
	} else {
		saved, e := loadCredential(c.base)
		if e != nil {
			return state, e
		}
		c.token = saved.Token
		if c.token != "" {
			state.Source = "saved"
		}
	}

	if c.token == "" {
		return state, errCheck
	}
	state.Username, state.ExpiresAt, err = tokenMetadata(c.token)
	if err != nil {
		return state, err
	}
	if !state.ExpiresAt.After(time.Now()) {
		return state, fmt.Errorf("token expired; run auth login: %w", errCheck)
	}
	if _, err := c.request(ctx, "GET", "/api/system/ai/providers", nil); err != nil {
		return state, err
	}
	state.Authenticated = true
	return state, nil
}

func readLoginPassword(ctx context.Context, o options, prompt io.Writer) (string, error) {
	if !o.passwordStdin {
		return terminalPassword(ctx, prompt)
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if len(raw) > 4096 {
		return "", errors.New("password too long")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if password == "" || strings.ContainsAny(password, "\r\n") {
		return "", errors.New("provide one nonempty password line")
	}
	return password, nil
}
