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
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/config"
)

const completionCacheTTL = 5 * time.Minute

type completionCache struct {
	GeneratedAt time.Time `json:"generated_at"`
	Names       []string  `json:"names"`
}

// secretNameCompletion is meant for cobra's ValidArgsFunction. It returns
// the list of secret names known to the configured backend so the user can
// tab-complete them. Network round-trips would make every TAB-press feel
// laggy, so the result is cached on disk for completionCacheTTL.
//
// Failures are silent: shell completion must not print errors to stderr
// (cobra echoes them into the user's shell) — we just return ShellCompDirectiveError.
func secretNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if names, ok := readCompletionCache(); ok {
		return names, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	b, err := buildBackend(cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	secrets, err := b.ListSecrets(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(secrets))
	for _, s := range secrets {
		names = append(names, s.Name)
	}
	_ = writeCompletionCache(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completionCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "jtsekret", "completion.json"), nil
}

func readCompletionCache() ([]string, bool) {
	path, err := completionCachePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var c completionCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false
	}
	if time.Since(c.GeneratedAt) > completionCacheTTL {
		return nil, false
	}
	return c.Names, true
}

func writeCompletionCache(names []string) error {
	path, err := completionCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(completionCache{GeneratedAt: time.Now(), Names: names})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".jtsekret-comp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	return os.Rename(tmpPath, path)
}

// invalidateCompletionCache wipes the on-disk completion cache. Call it
// from any command that creates/renames/deletes a secret so the next
// TAB-press reflects reality.
func invalidateCompletionCache() {
	path, err := completionCachePath()
	if err != nil {
		return
	}
	err = os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Best-effort — not worth surfacing to the user.
		return
	}
}
