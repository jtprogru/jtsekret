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

// Package audit appends one JSON line per privileged operation
// (read/create/update/delete/exec) to a per-user log. Only the action,
// secret name, and (optionally) entry key are recorded — never the value.
//
// The log lives under $XDG_STATE_HOME/jtsekret (or ~/.local/state/jtsekret)
// and is best-effort: failures are returned to the caller, who decides
// whether to surface them. The CLI logs failures to stderr at debug level
// and continues — losing audit lines must never break a secret operation.
package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one line in the audit log. Marshaled as JSON, one per line.
type Entry struct {
	Time    time.Time `json:"time"`
	Action  string    `json:"action"`
	Backend string    `json:"backend,omitempty"`
	Secret  string    `json:"secret,omitempty"`
	Key     string    `json:"key,omitempty"`
	Result  string    `json:"result"` // "ok" | "error"
	Error   string    `json:"error,omitempty"`
}

var (
	mu       sync.Mutex
	disabled bool
)

// Disable turns audit-logging off process-wide. Intended for tests, the
// `--no-audit` flag, or environments where the user explicitly disables
// the log.
func Disable() {
	mu.Lock()
	disabled = true
	mu.Unlock()
}

// Path returns the resolved audit log path, honouring XDG_STATE_HOME and
// falling back to ~/.local/state/jtsekret/audit.log.
func Path() (string, error) {
	if p := os.Getenv("JTSEKRET_AUDIT_LOG"); p != "" {
		return p, nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "jtsekret", "audit.log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve audit path: %w", err)
	}
	return filepath.Join(home, ".local", "state", "jtsekret", "audit.log"), nil
}

// Append writes one Entry to the audit log. Returns nil silently when
// auditing has been Disable()'d.
func Append(e Entry) error {
	mu.Lock()
	defer mu.Unlock()
	if disabled {
		return nil
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Result == "" {
		e.Result = "ok"
	}
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("audit mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit open: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(e) //nolint:gosec // G117 false positive: Entry.Secret holds the secret NAME, never its value
	if err != nil {
		return fmt.Errorf("audit marshal: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("audit write: %w", err)
	}
	return nil
}

// FromError derives Result and Error fields based on whether err is nil.
// Convenience wrapper around Append for the common "log success or
// failure of an op" pattern.
func FromError(action, backend, secret, key string, err error) error {
	e := Entry{Action: action, Backend: backend, Secret: secret, Key: key}
	if err != nil {
		e.Result = "error"
		e.Error = err.Error()
	}
	return Append(e)
}

// LastN returns the most recent n parsed entries, oldest first within the
// returned slice. If the log doesn't exist yet, returns an empty slice.
func LastN(n int) ([]Entry, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}
	// Split on '\n', trim trailing empty line. We don't bother with a
	// streaming parser — the personal-use log is bounded.
	var lines [][]byte
	start := 0
	for i, c := range data {
		if c == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]Entry, 0, len(lines))
	for _, l := range lines {
		var e Entry
		if err := json.Unmarshal(l, &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
