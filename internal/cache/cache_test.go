/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/domain"
)

const testPassword = "test-cache-password"

func newCache(t *testing.T) (*EncryptedFile, string) {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cache.enc")
	c, err := NewEncryptedFile(path, testPassword)
	if err != nil {
		t.Fatalf("NewEncryptedFile: %v", err)
	}
	return c, path
}

func samplePayload(value string) *domain.CachedPayload {
	return &domain.CachedPayload{
		Entries:   map[string][]byte{"k": []byte(value)},
		CachedAt:  time.Now(),
		TTL:       time.Hour,
		VersionID: "1",
	}
}

func TestEncryptedFile_SetGetRoundtrip(t *testing.T) {
	ctx := context.Background()
	c, _ := newCache(t)

	if err := c.Set(ctx, "alpha", samplePayload("AAA")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned (nil, nil); expected payload")
	}
	if string(got.Entries["k"]) != "AAA" {
		t.Fatalf("Entries[k] = %q, want AAA", got.Entries["k"])
	}
}

func TestEncryptedFile_GetMissing(t *testing.T) {
	ctx := context.Background()
	c, _ := newCache(t)
	got, err := c.Get(ctx, "nope")
	if err != nil {
		t.Fatalf("Get on missing key: err=%v", err)
	}
	if got != nil {
		t.Fatalf("Get on missing key: got %+v, want nil", got)
	}
}

func TestEncryptedFile_TTLExpiryReturnsMiss(t *testing.T) {
	ctx := context.Background()
	c, _ := newCache(t)
	// CachedAt 2h in the past, TTL 1h ⇒ expired.
	expired := &domain.CachedPayload{
		Entries:  map[string][]byte{"k": []byte("v")},
		CachedAt: time.Now().Add(-2 * time.Hour),
		TTL:      time.Hour,
	}
	if err := c.Set(ctx, "expired", expired); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "expired")
	if err != nil {
		t.Fatalf("Get on expired: err=%v", err)
	}
	if got != nil {
		t.Fatalf("Get on expired: got %+v, want nil (treated as miss)", got)
	}
	// Expired entry should also have been removed from disk.
	stats, err := c.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["total"].(int) != 0 {
		t.Fatalf("expired entry not purged: stats=%+v", stats)
	}
}

func TestEncryptedFile_PersistsAcrossInstances(t *testing.T) {
	ctx := context.Background()
	c, path := newCache(t)
	if err := c.Set(ctx, "persist", samplePayload("stored")); err != nil {
		t.Fatal(err)
	}

	// Re-open the same file with the same password — should round-trip.
	c2, err := NewEncryptedFile(path, testPassword)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	got, err := c2.Get(ctx, "persist")
	if err != nil || got == nil {
		t.Fatalf("Get after re-open: err=%v got=%+v", err, got)
	}
	if string(got.Entries["k"]) != "stored" {
		t.Fatalf("post-reopen value: %q", got.Entries["k"])
	}
}

func TestEncryptedFile_WrongPasswordFails(t *testing.T) {
	ctx := context.Background()
	c, path := newCache(t)
	if err := c.Set(ctx, "x", samplePayload("secret-value")); err != nil {
		t.Fatal(err)
	}

	if _, err := NewEncryptedFile(path, "wrong-password"); err == nil {
		t.Fatal("expected NewEncryptedFile with wrong password to fail GCM auth")
	}
}

func TestEncryptedFile_DeleteAndClear(t *testing.T) {
	ctx := context.Background()
	c, _ := newCache(t)
	for _, n := range []string{"a", "b", "c"} {
		if err := c.Set(ctx, n, samplePayload(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Delete(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get(ctx, "b"); got != nil {
		t.Fatal("Delete didn't remove entry")
	}
	stats, _ := c.Stats(ctx)
	if stats["total"].(int) != 2 {
		t.Fatalf("post-delete stats: %+v", stats)
	}
	if err := c.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	stats, _ = c.Stats(ctx)
	if stats["total"].(int) != 0 {
		t.Fatalf("post-clear stats: %+v", stats)
	}
}

func TestEncryptedFile_Stats(t *testing.T) {
	ctx := context.Background()
	c, _ := newCache(t)
	// Two valid + one expired entry.
	for _, n := range []string{"v1", "v2"} {
		if err := c.Set(ctx, n, samplePayload(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Set(ctx, "exp", &domain.CachedPayload{
		Entries:  map[string][]byte{"k": []byte("x")},
		CachedAt: time.Now().Add(-2 * time.Hour),
		TTL:      time.Hour,
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := c.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["total"].(int) != 3 {
		t.Fatalf("total=%v want 3", stats["total"])
	}
	if stats["valid"].(int) != 2 {
		t.Fatalf("valid=%v want 2", stats["valid"])
	}
	if stats["expired"].(int) != 1 {
		t.Fatalf("expired=%v want 1", stats["expired"])
	}
}

func TestEncryptedFile_CorruptedFile(t *testing.T) {
	c, path := newCache(t)
	if err := c.Set(context.Background(), "x", samplePayload("v")); err != nil {
		t.Fatal(err)
	}
	// Truncate the cache file to a length below the salt+nonce header.
	if err := os.WriteFile(path, []byte("not-a-cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEncryptedFile(path, testPassword); err == nil {
		t.Fatal("expected NewEncryptedFile to reject a truncated cache file")
	}
}

func TestNewCache_DisabledReturnsNoop(t *testing.T) {
	got, err := NewCache(context.Background(), config.CacheConfig{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(*Noop); !ok {
		t.Fatalf("disabled cache: got %T, want *Noop", got)
	}
	// Noop always reports a miss.
	v, err := got.Get(context.Background(), "x")
	if err != nil || v != nil {
		t.Fatalf("Noop.Get: %v / %+v", err, v)
	}
}
