/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package clipboard

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBuildCommand_PrefersPlatformTool exercises the runtime.GOOS branch
// selection without actually launching the clipboard binary. We verify
// that, on the current host, buildCommand either returns a non-nil cmd
// (whose Path matches one of the documented tools) or surfaces ErrNoTool.
func TestBuildCommand_PicksKnownTool(t *testing.T) {
	cmd, err := buildCommand(context.Background())
	if err != nil {
		if !errors.Is(err, ErrNoTool) {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Skipf("no clipboard tool installed on this host (GOOS=%s)", runtime.GOOS)
	}
	binary := filepath.Base(cmd.Path)
	allowed := map[string]struct{}{
		"pbcopy":  {},
		"wl-copy": {},
		"xclip":   {},
		"xsel":    {},
	}
	if _, ok := allowed[binary]; !ok {
		t.Fatalf("buildCommand picked unexpected binary %q", binary)
	}
}

func TestCopy_RoundTrip(t *testing.T) {
	// We don't want a unit test to actually scribble on the user's
	// clipboard — but we do want to verify the wire-up doesn't blow up
	// for trivial inputs. If no tool is available on the host, skip.
	if _, err := buildCommand(context.Background()); err != nil {
		if errors.Is(err, ErrNoTool) {
			t.Skip("no clipboard tool installed")
		}
		t.Fatal(err)
	}
	if err := Copy(context.Background(), []byte("test-clipboard-value")); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if err := Clear(context.Background()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
}
