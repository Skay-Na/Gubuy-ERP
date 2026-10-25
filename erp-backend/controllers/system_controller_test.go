package controllers

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestMySQLClientArgsSelectTLSOption(t *testing.T) {
	t.Setenv("DB_NAME", "backup_test")
	t.Setenv("DB_PASS", "")
	cases := []struct {
		name       string
		binary     string
		wantOption string
	}{
		{name: "mariadb", binary: "mariadb-dump", wantOption: "--skip-ssl"},
		{name: "mysql", binary: "mysqldump", wantOption: "--ssl-mode=DISABLED"},
		{name: "docker mysql client", binary: "mysql", wantOption: "--ssl-mode=DISABLED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := mysqlClientCommand{
				execPath: filepath.Join(t.TempDir(), tc.binary),
				binary:   tc.binary,
				host:     "127.0.0.1",
				port:     "3306",
			}
			args := mysqlClientArgs(client)
			if !slices.Contains(args, tc.wantOption) {
				t.Fatalf("args %v lack required client option %q", args, tc.wantOption)
			}
			if args[len(args)-1] != "backup_test" {
				t.Fatalf("database must follow options, got %v", args)
			}
		})
	}
}

func TestMySQLClientCommandFullArgs(t *testing.T) {
	local := mysqlClientCommand{execPath: "mysql", binary: "mysql", host: "127.0.0.1", port: "3306"}
	if got := local.fullArgs([]string{"-h", "127.0.0.1"}); len(got) != 2 {
		t.Fatalf("local client must not add wrapper args, got %v", got)
	}

	docker := mysqlClientCommand{execPath: "docker", prefix: []string{"exec", "-i", "gubuy-erp-db"}, binary: "mysql", host: "127.0.0.1", port: "3306"}
	got := docker.fullArgs([]string{"erp_db"})
	want := []string{"exec", "-i", "gubuy-erp-db", "mysql", "erp_db"}
	if !slices.Equal(got, want) {
		t.Fatalf("fullArgs = %v, want %v", got, want)
	}
}

func TestFindMySQLCommand(t *testing.T) {
	// 本地 PATH 查找必须保持最高优先级，且不依赖 Docker 环境
	t.Setenv("DB_CLIENT_MODE", "local")
	cases := []struct {
		name   string
		binary string
	}{
		{name: "current mariadb name", binary: "mariadb-dump"},
		{name: "mysql fallback", binary: "mysqldump"},
		{name: "missing clients"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			filename := tc.binary
			if runtime.GOOS == "windows" && filename != "" {
				// Windows 的 LookPath 只认带 PATHEXT 扩展名的可执行文件
				filename += ".exe"
			}
			if tc.binary != "" {
				if err := os.WriteFile(filepath.Join(dir, filename), []byte("#!/bin/sh\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			command, err := findMySQLCommand("mariadb-dump", "mysqldump")
			if tc.binary == "" {
				if err == nil {
					t.Fatal("missing client must return an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if command.binary != tc.binary {
				t.Fatalf("client binary = %q, want %q", command.binary, tc.binary)
			}
			if command.execPath != filepath.Join(dir, filename) {
				t.Fatalf("execPath = %q, want %q", command.execPath, filepath.Join(dir, filename))
			}
		})
	}
}

func TestFindDockerMySQLClientRequiresDocker(t *testing.T) {
	t.Setenv("DB_CLIENT_MODE", "auto")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed, skip docker fallback test")
	}
	t.Setenv("DB_DOCKER_CONTAINER", "definitely-not-running-container")
	if _, err := findDockerMySQLClient("mysql"); err == nil {
		t.Fatal("missing container must return an error")
	}
}
