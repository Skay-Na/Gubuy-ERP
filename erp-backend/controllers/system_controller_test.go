package controllers

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMySQLClientArgsSelectTLSOption(t *testing.T) {
	t.Setenv("DB_NAME", "backup_test")
	t.Setenv("DB_PASS", "")
	cases := []struct {
		name       string
		binary     string
		alias      string
		wantOption string
	}{
		{name: "mariadb", binary: "mariadb-dump", wantOption: "--skip-ssl"},
		{name: "mysql", binary: "mysqldump", wantOption: "--ssl-mode=DISABLED"},
		{name: "mariadb legacy alias", binary: "mariadb-dump", alias: "mysqldump", wantOption: "--skip-ssl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			command := filepath.Join(dir, tc.binary)
			if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.alias != "" {
				alias := filepath.Join(dir, tc.alias)
				if err := os.Symlink(command, alias); err != nil {
					t.Fatal(err)
				}
				command = alias
			}
			args := mysqlClientArgs(command)
			if !slices.Contains(args, tc.wantOption) {
				t.Fatalf("args %v lack required client option %q", args, tc.wantOption)
			}
			if args[len(args)-1] != "backup_test" {
				t.Fatalf("database must follow options, got %v", args)
			}
		})
	}
}

func TestFindMySQLCommand(t *testing.T) {
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
			if tc.binary != "" {
				if err := os.WriteFile(filepath.Join(dir, tc.binary), []byte("#!/bin/sh\n"), 0o700); err != nil {
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
			if command != filepath.Join(dir, tc.binary) {
				t.Fatalf("command = %q, want %q", command, tc.binary)
			}
		})
	}
}
