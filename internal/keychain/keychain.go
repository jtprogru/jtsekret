/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

// Package keychain wraps the macOS `security` CLI to read/write generic
// passwords. We shell out instead of pulling in a CGO+Security.framework
// dependency: the slots we care about are tiny (one master password per
// backend), and Apple's tool is universally available, signed, and
// honours the user's policy (Touch ID, "always allow", etc).
//
// On non-Darwin platforms every operation returns ErrUnsupported and
// callers fall back to the env-var path.
package keychain

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Service identifies all jtsekret entries in the user's Keychain. Account
// names are the slot names (cache / github / file).
const Service = "jtsekret"

// ErrUnsupported is returned on platforms without macOS Keychain (any
// non-darwin GOOS). Callers treat this as "fall back to env var".
var ErrUnsupported = errors.New("keychain: not supported on this OS")

// ErrNotFound is returned by Get when no entry exists for the slot.
var ErrNotFound = errors.New("keychain: entry not found")

// KnownSlots are the slots jtsekret manages. Used by `keychain list` and
// validated against in `keychain set/get/unset` to keep typos out of the
// user's keychain.
var KnownSlots = []string{"cache", "github", "file"}

// Available reports whether keychain operations will work in the current
// environment. Useful for `config show` and conditional fallbacks.
func Available() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("security")
	return err == nil
}

// Get fetches the password for slot from the user's Keychain.
func Get(ctx context.Context, slot string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", ErrUnsupported
	}
	if _, err := exec.LookPath("security"); err != nil {
		return "", fmt.Errorf("keychain: `security` CLI not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-a", slot,
		"-s", Service,
		"-w") // -w prints just the password
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// `security` returns 44 (errSecItemNotFound) when the item
			// doesn't exist; surface a friendly error so callers can
			// distinguish "missing" from "broken keychain".
			stderr := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(stderr, "could not be found") {
				return "", ErrNotFound
			}
			return "", fmt.Errorf("keychain get %q: %s", slot, stderr)
		}
		return "", fmt.Errorf("keychain get %q: %w", slot, err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Set writes (or overwrites) the password for slot. -U makes the call
// idempotent: existing entries are updated, new ones are created.
func Set(ctx context.Context, slot, password string) error {
	if runtime.GOOS != "darwin" {
		return ErrUnsupported
	}
	if _, err := exec.LookPath("security"); err != nil {
		return fmt.Errorf("keychain: `security` CLI not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, "security", "add-generic-password",
		"-a", slot,
		"-s", Service,
		"-w", password,
		"-U")
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return fmt.Errorf("keychain set %q: %s", slot, strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("keychain set %q: %w", slot, err)
	}
	return nil
}

// Delete removes slot. Returns ErrNotFound if the entry doesn't exist.
func Delete(ctx context.Context, slot string) error {
	if runtime.GOOS != "darwin" {
		return ErrUnsupported
	}
	if _, err := exec.LookPath("security"); err != nil {
		return fmt.Errorf("keychain: `security` CLI not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, "security", "delete-generic-password",
		"-a", slot,
		"-s", Service)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(stderr, "could not be found") {
				return ErrNotFound
			}
			return fmt.Errorf("keychain delete %q: %s", slot, stderr)
		}
		return fmt.Errorf("keychain delete %q: %w", slot, err)
	}
	return nil
}

// List returns the subset of KnownSlots that currently exist in the
// keychain. Errors other than ErrNotFound short-circuit and propagate.
func List(ctx context.Context) ([]string, error) {
	if !Available() {
		return nil, ErrUnsupported
	}
	present := make([]string, 0, len(KnownSlots))
	for _, slot := range KnownSlots {
		_, err := Get(ctx, slot)
		switch {
		case err == nil:
			present = append(present, slot)
		case errors.Is(err, ErrNotFound):
			continue
		default:
			return nil, err
		}
	}
	return present, nil
}

// IsKnownSlot guards against typos when accepting user input.
func IsKnownSlot(s string) bool {
	for _, k := range KnownSlots {
		if k == s {
			return true
		}
	}
	return false
}
