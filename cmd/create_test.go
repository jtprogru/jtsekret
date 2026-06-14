/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/viper"

	// Blank import registers the mock backend so buildBackend can resolve
	// backend.type = "mock" through the global registry.
	_ "github.com/jtprogru/jtsekret/internal/backend/mock"
)

// useMockBackend points config.Load() at the in-memory mock backend for the
// duration of a test by setting the process-global viper key, then restores it.
func useMockBackend(t *testing.T) {
	t.Helper()
	prev := viper.Get("backend.type")
	viper.Set("backend.type", "mock")
	t.Cleanup(func() { viper.Set("backend.type", prev) })
}

// setCreateFlags sets the package-global create flags and restores them.
func setCreateFlags(t *testing.T, key, value string) {
	t.Helper()
	prevKey, prevVal := createKey, createValue
	createKey, createValue = key, value
	t.Cleanup(func() { createKey, createValue = prevKey, prevVal })
}

// runCreate must reject an unsafe entry key even when the backend is reachable,
// so the validation runs through the real create path against the mock backend.
func TestRunCreate_RejectsUnsafeKey(t *testing.T) {
	useMockBackend(t)

	for _, key := range []string{"../evil", "a/b", "..", `a\b`} {
		setCreateFlags(t, key, "value")
		err := runCreate(nil, []string{"secret"})
		if err == nil {
			t.Errorf("runCreate with key %q = nil, want error", key)
			continue
		}
		if !strings.Contains(err.Error(), "entry key") {
			t.Errorf("runCreate with key %q: error = %v, want entry-key validation error", key, err)
		}
	}
}

// A safe key (and no key at all) must pass validation and create the secret
// through the mock backend.
func TestRunCreate_AcceptsSafeKey(t *testing.T) {
	useMockBackend(t)

	setCreateFlags(t, "id_rsa", "value")
	if err := runCreate(nil, []string{"with-key"}); err != nil {
		t.Fatalf("runCreate with safe key: %v", err)
	}

	setCreateFlags(t, "", "")
	if err := runCreate(nil, []string{"no-key"}); err != nil {
		t.Fatalf("runCreate without key: %v", err)
	}
}
