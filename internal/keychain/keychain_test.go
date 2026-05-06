/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package keychain

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestIsKnownSlot(t *testing.T) {
	for _, ok := range KnownSlots {
		if !IsKnownSlot(ok) {
			t.Errorf("IsKnownSlot(%q) = false, want true", ok)
		}
	}
	bad := []string{"", "lockbox", "vault", "Cache", "github "}
	for _, b := range bad {
		if IsKnownSlot(b) {
			t.Errorf("IsKnownSlot(%q) = true, want false", b)
		}
	}
}

func TestUnsupportedOnNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin path tested by TestKeychainRoundTrip")
	}
	if Available() {
		t.Fatalf("Available() = true on %s, want false", runtime.GOOS)
	}
	ctx := context.Background()
	if _, err := Get(ctx, "cache"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Get on non-darwin: got %v, want ErrUnsupported", err)
	}
	if err := Set(ctx, "cache", "x"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Set on non-darwin: got %v, want ErrUnsupported", err)
	}
	if err := Delete(ctx, "cache"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Delete on non-darwin: got %v, want ErrUnsupported", err)
	}
	if _, err := List(ctx); !errors.Is(err, ErrUnsupported) {
		t.Errorf("List on non-darwin: got %v, want ErrUnsupported", err)
	}
}

// TestKeychainRoundTrip exercises set/get/delete against the real macOS
// Keychain on darwin runners. Uses a unique slot name so it doesn't
// stomp on a real user's "cache"/"github"/"file" entries.
//
// Note: this isn't a known slot, so we don't go through the public
// IsKnownSlot guard — we call the package functions directly. On a
// fresh login there may be a permission prompt; CI runners that lack
// a default keychain just skip via the Available() gate.
func TestKeychainRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	if !Available() {
		t.Skip("security CLI not available on this host")
	}
	const testSlot = "jtsekret-unit-test"
	ctx := context.Background()
	// Best-effort cleanup before and after.
	_ = Delete(ctx, testSlot)
	t.Cleanup(func() { _ = Delete(ctx, testSlot) })

	// Get on missing -> ErrNotFound.
	if _, err := Get(ctx, testSlot); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on missing: got %v, want ErrNotFound", err)
	}
	// Set then Get.
	if err := Set(ctx, testSlot, "secret-value-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := Get(ctx, testSlot)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != "secret-value-1" {
		t.Fatalf("Get = %q, want secret-value-1", got)
	}
	// Set again with -U updates in place.
	if err := Set(ctx, testSlot, "secret-value-2"); err != nil {
		t.Fatalf("Set update: %v", err)
	}
	got, err = Get(ctx, testSlot)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-value-2" {
		t.Fatalf("Get after update = %q, want secret-value-2", got)
	}
	// Delete then verify.
	if err := Delete(ctx, testSlot); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Get(ctx, testSlot); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
	if err := Delete(ctx, testSlot); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete on missing: got %v, want ErrNotFound", err)
	}
}
