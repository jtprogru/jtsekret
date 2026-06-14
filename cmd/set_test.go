/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package cmd

import (
	"strings"
	"testing"
)

// runSet validates the entry key before touching config or the backend, so a
// traversal key must be rejected up front without any backend wiring.
func TestRunSet_RejectsUnsafeKey(t *testing.T) {
	for _, key := range []string{"../evil", "a/b", "..", `a\b`, ""} {
		err := runSet(nil, []string{"secret", key, "value"})
		if err == nil {
			t.Errorf("runSet with key %q = nil, want error", key)
			continue
		}
		if !strings.Contains(err.Error(), "entry key") {
			t.Errorf("runSet with key %q: error = %v, want entry-key validation error", key, err)
		}
	}
}
