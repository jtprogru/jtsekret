/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/

//go:build integration

// Live integration tests against a real `vault server -dev` process.
// Run with: `go test -tags=integration ./internal/backend/vault/...`
//
// Requires the `vault` binary on PATH (https://developer.hashicorp.com/
// vault/install). The test spawns a short-lived dev server on a random
// localhost port, runs CRUD against it, then kills the process.
package vault

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
)

// freePort grabs an unused TCP port by listening on :0 and immediately
// closing — a small race window remains, but it's the standard pattern
// for dev-server tests and good enough for personal-scale CI.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	_ = l.Close()
	return addr.String()
}

func startDevVault(t *testing.T) (addr, token string) {
	t.Helper()
	if _, err := exec.LookPath("vault"); err != nil {
		t.Skipf("vault binary not on PATH: %v", err)
	}
	const rootToken = "dev-root-token-jtsekret"
	listen := freePort(t)
	cmd := exec.Command("vault", "server", "-dev",
		"-dev-root-token-id="+rootToken,
		"-dev-listen-address="+listen)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start vault: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	url := "http://" + listen
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// Vault exposes /v1/sys/health unauthenticated; once it returns
		// 200 (initialised+unsealed+active), the dev server is ready.
		resp, err := http.Get(url + "/v1/sys/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return url, rootToken
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for vault dev server at %s", url)
	return "", ""
}

func TestIntegration_VaultCRUD(t *testing.T) {
	addr, token := startDevVault(t)
	cfg := map[string]interface{}{
		"address": addr,
		"mount":   "secret",
		"prefix":  "jtsekret-it",
		"auth":    map[string]interface{}{"type": "token", "token": token},
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	name := "demo-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000"), ".", "")

	if _, err := b.CreateSecret(ctx, name, "integration-test", []backend.Entry{
		{Key: "user", Value: []byte("alice")},
		{Key: "tok", Value: []byte("abc-123")},
	}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	list, err := b.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, s := range list {
		if s.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("List didn't include the created secret %q: %+v", name, list)
	}

	p, err := b.GetPayload(ctx, name, "")
	if err != nil {
		t.Fatalf("GetPayload v1: %v", err)
	}
	if p.VersionID != "1" {
		t.Fatalf("first version: %q want 1", p.VersionID)
	}

	if err := b.AddVersion(ctx, name, []backend.Entry{
		{Key: "tok", Value: []byte("rotated")},
	}); err != nil {
		t.Fatalf("AddVersion: %v", err)
	}
	p, _ = b.GetPayload(ctx, name, "")
	if p.VersionID != "2" || string(p.Entries[0].Value) != "rotated" {
		t.Fatalf("after rotate: %+v", p)
	}
	pV1, err := b.GetPayload(ctx, name, "1")
	if err != nil {
		t.Fatalf("GetPayload v1 by version: %v", err)
	}
	if got := stringMap(pV1.Entries)["user"]; got != "alice" {
		t.Fatalf("v1 didn't retain original entries: %+v", pV1.Entries)
	}

	if err := b.DeleteSecret(ctx, name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := b.GetPayload(ctx, name, ""); err == nil {
		t.Fatal("GetPayload after Delete should fail")
	}
}

func TestIntegration_VaultBadToken(t *testing.T) {
	addr, _ := startDevVault(t)
	cfg := map[string]interface{}{
		"address": addr,
		"mount":   "secret",
		"auth":    map[string]interface{}{"type": "token", "token": "wrong-token"},
	}
	// `New` doesn't itself validate the token — first real call does.
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("New with bad token: %v", err)
	}
	if _, err := b.ListSecrets(context.Background()); err == nil || !strings.Contains(err.Error(), "permission") {
		// Vault returns 403 on bad token; allow either "permission denied"
		// or any other errored-out variant — what matters is it's not nil.
		if err == nil {
			t.Fatal("expected ListSecrets to fail with bad token")
		}
		// Don't be picky about the message text across vault versions;
		// any error is fine for this assertion.
		var execErr *exec.ExitError
		if errors.As(err, &execErr) {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
}

func stringMap(entries []backend.Entry) map[string]string {
	out := map[string]string{}
	for _, e := range entries {
		out[e.Key] = string(e.Value)
	}
	return out
}
