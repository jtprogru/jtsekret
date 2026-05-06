/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/

//go:build integration

// Live integration tests against a real Yandex Cloud Lockbox folder.
// Run with:
//
//	JTSEKRET_TEST_FOLDER_ID=b1g... \
//	go test -tags=integration ./internal/backend/lockbox/...
//
// Auth is resolved by the production code path (auth.type=auto): the
// `yc` CLI must be authenticated, or YC_IAM_TOKEN / YC_OAUTH_TOKEN /
// YC_SERVICE_ACCOUNT_KEY_FILE must be set.
package lockbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
)

func skipIfNoFolder(t *testing.T) string {
	t.Helper()
	folder := os.Getenv("JTSEKRET_TEST_FOLDER_ID")
	if folder == "" {
		t.Skip("JTSEKRET_TEST_FOLDER_ID not set; skipping live lockbox integration")
	}
	return folder
}

func newIntegrationBackend(t *testing.T) backend.Backend {
	t.Helper()
	folder := skipIfNoFolder(t)
	cfg := map[string]interface{}{
		"folder_id": folder,
		"auth":      map[string]interface{}{"type": "auto"},
	}
	b, err := NewBackend(cfg)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	return b
}

// uniqueName returns a name that won't collide with anything else in the
// folder, even with parallel runs.
func uniqueName(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func TestIntegration_LockboxRoundTrip(t *testing.T) {
	b := newIntegrationBackend(t)
	ctx := context.Background()
	name := uniqueName(t, "jtsekret-it")

	// Cleanup tries to run even if the test fails partway through.
	t.Cleanup(func() {
		// Resolve via name; might already be gone if the test deleted it.
		if err := b.DeleteSecret(context.Background(), name); err != nil {
			if !strings.Contains(err.Error(), "not found") {
				t.Logf("cleanup: %v", err)
			}
		}
	})

	created, err := b.CreateSecret(ctx, name, "jtsekret integration test", []backend.Entry{
		{Key: "user", Value: []byte("alice")},
		{Key: "tok", Value: []byte("abc-123")},
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if !secretIDPattern.MatchString(created.ID) {
		t.Fatalf("created secret has unexpected ID format: %q", created.ID)
	}

	list, err := b.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	found := false
	for _, s := range list {
		if s.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created secret %q not in list", name)
	}

	// Get by name (resolveID exercise).
	p, err := b.GetPayload(ctx, name, "")
	if err != nil {
		t.Fatalf("GetPayload by name: %v", err)
	}
	got := stringMap(p.Entries)
	if got["user"] != "alice" || got["tok"] != "abc-123" {
		t.Fatalf("payload mismatch: %+v", got)
	}

	if err := b.AddVersion(ctx, name, []backend.Entry{
		{Key: "tok", Value: []byte("rotated")},
	}); err != nil {
		t.Fatalf("AddVersion: %v", err)
	}
	p, _ = b.GetPayload(ctx, name, "")
	got = stringMap(p.Entries)
	if got["tok"] != "rotated" {
		t.Fatalf("after AddVersion: %+v", got)
	}

	if err := b.DeleteSecret(ctx, name); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if _, err := b.GetSecret(ctx, name); err == nil {
		t.Fatal("GetSecret after Delete should fail")
	}
}

func TestIntegration_LockboxResolveID_AmbiguousAndMissing(t *testing.T) {
	b := newIntegrationBackend(t)
	ctx := context.Background()
	missing := uniqueName(t, "definitely-nonexistent")

	if _, err := b.GetSecret(ctx, missing); err == nil {
		t.Fatal("GetSecret on missing should fail")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found, got %v", err)
	}
	// Sanity: a clearly non-ID input shouldn't accidentally hit the regex.
	if secretIDPattern.MatchString(missing) {
		t.Fatalf("regex incorrectly matched %q as an ID", missing)
	}
}

func stringMap(entries []backend.Entry) map[string]string {
	out := map[string]string{}
	for _, e := range entries {
		out[e.Key] = string(e.Value)
	}
	return out
}

// Sanity: ensure errors package is wired into the test binary so future
// edits that need errors.Is/errors.As don't have to add the import.
var _ = errors.New
