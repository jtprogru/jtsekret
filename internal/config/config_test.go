/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromFile_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "jtsekret.yaml")
	yaml := `
backend:
  type: github
  github:
    repo: "owner/repo"
    branch: main
    local_path: "/tmp/store"
    auto_pull: false
    auto_push: true
    auth:
      type: token
      token: "ghp_xxx"
cache:
  enabled: true
  ttl: 600
  path: "/tmp/cache.enc"
output:
  format: json
log:
  level: debug
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JTSEKRET_CACHE_MASTER_PASSWORD", "")
	t.Setenv("JTSEKRET_GITHUB_MASTER_PASSWORD", "")
	t.Setenv("JTSEKRET_FILE_MASTER_PASSWORD", "")
	t.Setenv("JTSEKRET_GITHUB_TOKEN", "")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.Backend.Type != "github" || cfg.Backend.Github.Repo != "owner/repo" {
		t.Fatalf("backend: %+v", cfg.Backend)
	}
	if cfg.Backend.Github.Auth.Type != "token" || cfg.Backend.Github.Auth.Token != "ghp_xxx" {
		t.Fatalf("auth: %+v", cfg.Backend.Github.Auth)
	}
	if !cfg.Cache.Enabled || cfg.Cache.TTL != 600 {
		t.Fatalf("cache: %+v", cfg.Cache)
	}
	if cfg.Output.GetFormat() != "json" {
		t.Fatalf("output: %+v", cfg.Output)
	}
	if cfg.Log.GetLevel() != "debug" {
		t.Fatalf("log: %+v", cfg.Log)
	}
}

func TestLoadFromFile_Missing(t *testing.T) {
	_, err := LoadFromFile("/no/such/path.yaml")
	if err == nil {
		t.Fatal("expected error on missing file")
	}
}

func TestLoadFromFile_EnvOverridesFillsTokens(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.yaml")
	yaml := `
backend:
  type: github
  github:
    repo: "x/y"
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JTSEKRET_GITHUB_TOKEN", "from-env")
	t.Setenv("VAULT_ADDR", "https://vault.example:8200")
	t.Setenv("VAULT_TOKEN", "tok-vault")

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend.Github.Auth.Token != "from-env" {
		t.Fatalf("github token: %q", cfg.Backend.Github.Auth.Token)
	}
	if cfg.Backend.Vault.Address != "https://vault.example:8200" {
		t.Fatalf("vault addr: %q", cfg.Backend.Vault.Address)
	}
	if cfg.Backend.Vault.Auth.Token != "tok-vault" {
		t.Fatalf("vault token: %q", cfg.Backend.Vault.Auth.Token)
	}
}

func TestMasterPassword_FallbackToCache(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.yaml")
	if err := os.WriteFile(path, []byte("backend:\n  type: github\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JTSEKRET_CACHE_MASTER_PASSWORD", "shared-pass")
	t.Setenv("JTSEKRET_GITHUB_MASTER_PASSWORD", "")
	t.Setenv("JTSEKRET_FILE_MASTER_PASSWORD", "")

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend.Github.MasterPassword != "shared-pass" {
		t.Fatalf("github master password: %q", cfg.Backend.Github.MasterPassword)
	}
	if cfg.Backend.File.MasterPassword != "shared-pass" {
		t.Fatalf("file master password: %q", cfg.Backend.File.MasterPassword)
	}
}

func TestMasterPassword_BackendOverridesCache(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.yaml")
	if err := os.WriteFile(path, []byte("backend:\n  type: github\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JTSEKRET_CACHE_MASTER_PASSWORD", "cache")
	t.Setenv("JTSEKRET_GITHUB_MASTER_PASSWORD", "github-specific")
	t.Setenv("JTSEKRET_FILE_MASTER_PASSWORD", "")

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend.Github.MasterPassword != "github-specific" {
		t.Fatalf("got %q", cfg.Backend.Github.MasterPassword)
	}
	if cfg.Backend.File.MasterPassword != "cache" {
		t.Fatalf("file falls back to cache: %q", cfg.Backend.File.MasterPassword)
	}
}

func TestPathExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()

	cc := CacheConfig{Path: "~/cache"}
	c := &Config{Cache: cc}
	if got, want := c.GetCachePath(), filepath.Join(home, "cache"); got != want {
		t.Fatalf("GetCachePath: got %q want %q", got, want)
	}
	abs := CacheConfig{Path: "/abs/path"}
	c = &Config{Cache: abs}
	if got := c.GetCachePath(); got != "/abs/path" {
		t.Fatalf("absolute path got mangled: %q", got)
	}

	gh := GithubConfig{}
	if got := gh.GetLocalPath(); !strings.HasPrefix(got, home) {
		t.Fatalf("default github local_path didn't expand ~: %q", got)
	}
	gh = GithubConfig{LocalPath: "/some/abs"}
	if got := gh.GetLocalPath(); got != "/some/abs" {
		t.Fatalf("absolute path: %q", got)
	}

	fc := FileConfig{}
	if got := fc.GetPath(); !strings.HasPrefix(got, home) {
		t.Fatalf("default file path didn't expand ~: %q", got)
	}
}

func TestGetFormat(t *testing.T) {
	cases := map[string]string{
		"table":   "table",
		"json":    "json",
		"plain":   "plain",
		"":        "plain",
		"weird":   "plain",
	}
	for in, want := range cases {
		oc := OutputConfig{Format: in}
		if got := oc.GetFormat(); got != want {
			t.Fatalf("GetFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetLogLevel(t *testing.T) {
	cases := map[string]string{
		"debug": "debug",
		"info":  "info",
		"warn":  "warn",
		"error": "error",
		"":      "warn",
		"junk":  "warn",
	}
	for in, want := range cases {
		lc := LogConfig{Level: in}
		if got := lc.GetLevel(); got != want {
			t.Fatalf("GetLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	// Missing backend type.
	err := Validate(&Config{})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "backend.type" {
		t.Fatalf("expected backend.type ValidationError, got %v", err)
	}

	// Lockbox without folder_id.
	err = Validate(&Config{Backend: BackendConfig{Type: "lockbox"}})
	if !errors.As(err, &ve) || ve.Field != "backend.lockbox.folder_id" {
		t.Fatalf("expected folder_id error, got %v", err)
	}

	// Lockbox with folder_id, default auth type filled in.
	cfg := &Config{Backend: BackendConfig{Type: "lockbox", Lockbox: LockboxConfig{FolderID: "f"}}}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Backend.Lockbox.Auth.Type != "oauth" {
		t.Fatalf("default auth.type: %q", cfg.Backend.Lockbox.Auth.Type)
	}

	// Cache enabled but path empty.
	cfg = &Config{
		Backend: BackendConfig{Type: "lockbox", Lockbox: LockboxConfig{FolderID: "f"}},
		Cache:   CacheConfig{Enabled: true},
	}
	err = Validate(cfg)
	if !errors.As(err, &ve) || ve.Field != "cache.path" {
		t.Fatalf("expected cache.path error, got %v", err)
	}

	// Cache enabled, path set, no master password and env empty -> error.
	t.Setenv("JTSEKRET_CACHE_MASTER_PASSWORD", "")
	cfg = &Config{
		Backend: BackendConfig{Type: "lockbox", Lockbox: LockboxConfig{FolderID: "f"}},
		Cache:   CacheConfig{Enabled: true, Path: "/x"},
	}
	err = Validate(cfg)
	if !errors.As(err, &ve) || ve.Field != "cache.master_password" {
		t.Fatalf("expected master_password error, got %v", err)
	}

	// Cache enabled with master password set -> ok.
	cfg = &Config{
		Backend: BackendConfig{Type: "lockbox", Lockbox: LockboxConfig{FolderID: "f"}},
		Cache:   CacheConfig{Enabled: true, Path: "/x", MasterPassword: "p"},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
