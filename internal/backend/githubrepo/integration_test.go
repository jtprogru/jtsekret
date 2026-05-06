/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/

//go:build integration

// Live integration tests for the github backend. Two modes:
//
//   - file:// bare repo (default): always runs, end-to-end through the
//     git layer with no network round-trip.
//   - real GitHub PAT (opt-in): set JTSEKRET_TEST_GITHUB_REPO=owner/repo
//     and JTSEKRET_TEST_GITHUB_TOKEN to exercise the live HTTPS push path.
//     Repo MUST be a throwaway one — the test creates and deletes secrets.
package githubrepo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
)

func TestIntegration_GithubFileRemoteFullCycle(t *testing.T) {
	// Same fixture used by the unit tests — bare repo over file:// — but
	// run as part of the integration tag so it's covered by `make test-it`
	// alongside the network-dependent backends.
	remote := makeBareRemote(t, "main")
	b := newBackend(t, remote, "main")
	ctx := context.Background()

	const name = "integration-secret"
	if _, err := b.CreateSecret(ctx, name, "integration", []backend.Entry{
		{Key: "user", Value: []byte("alice")},
		{Key: "tok", Value: []byte("abc-123")},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := b.AddVersion(ctx, name, []backend.Entry{
		{Key: "tok", Value: []byte("rotated")},
	}); err != nil {
		t.Fatalf("AddVersion: %v", err)
	}
	if err := b.RotateMasterPassword(ctx, "new-pass"); err != nil {
		t.Fatalf("RotateMasterPassword: %v", err)
	}
	if err := b.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Re-clone from the same remote into a fresh worktree, with the
	// rotated password — should see the latest secret state.
	tmp := t.TempDir()
	cfg := map[string]interface{}{
		"repo":            remote,
		"branch":          "main",
		"local_path":      filepath.Join(tmp, "fresh"),
		"auto_pull":       true,
		"auto_push":       false,
		"master_password": "new-pass",
		"auth":            map[string]interface{}{"type": "none"},
	}
	fresh, err := New(cfg)
	if err != nil {
		t.Fatalf("clone fresh: %v", err)
	}
	p, err := fresh.GetPayload(ctx, name, "")
	if err != nil {
		t.Fatalf("GetPayload on fresh clone: %v", err)
	}
	if string(p.Entries[0].Value) != "rotated" {
		t.Fatalf("post-rotation value: %q", p.Entries[0].Value)
	}

	if err := b.DeleteSecret(ctx, name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestIntegration_GithubLivePAT(t *testing.T) {
	repo := os.Getenv("JTSEKRET_TEST_GITHUB_REPO")
	tok := os.Getenv("JTSEKRET_TEST_GITHUB_TOKEN")
	if repo == "" || tok == "" {
		t.Skip("set JTSEKRET_TEST_GITHUB_REPO and JTSEKRET_TEST_GITHUB_TOKEN to exercise the live HTTPS path")
	}
	tmp := t.TempDir()
	cfg := map[string]interface{}{
		"repo":            repo,
		"branch":          "main",
		"local_path":      filepath.Join(tmp, "live"),
		"auto_pull":       true,
		"auto_push":       true,
		"master_password": "integration-test-pass",
		"auth":            map[string]interface{}{"type": "token", "token": tok},
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	name := "jtsekret-it-" + time.Now().UTC().Format("150405")

	t.Cleanup(func() {
		if err := b.DeleteSecret(context.Background(), name); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})

	if _, err := b.CreateSecret(ctx, name, "live test", []backend.Entry{
		{Key: "tok", Value: []byte("live-value")},
	}); err != nil {
		t.Fatalf("CreateSecret on live repo: %v", err)
	}
	p, err := b.GetPayload(ctx, name, "")
	if err != nil {
		t.Fatalf("GetPayload: %v", err)
	}
	if string(p.Entries[0].Value) != "live-value" {
		t.Fatalf("payload mismatch: %q", p.Entries[0].Value)
	}
}
