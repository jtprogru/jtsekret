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
	"errors"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/audit"
	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/cache"
	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/domain"
)

var (
	dumpKey     string
	dumpDir     string
	dumpOutput  string
	dumpMode    string
	dumpForce   bool
	dumpNoCache bool
)

var dumpCmd = &cobra.Command{
	Use:   "dump <name>",
	Short: "Save secret entries to files",
	Long: `Save secret entries to files on disk.

By default, each entry key becomes a filename inside --dir.
With --key and --output you can save a single entry to a specific path.
Use --output - to write a single entry to stdout.

Examples:
  # Save all keys of secret "ssh-keys" to ~/.ssh/
  jtsekret dump ssh-keys --dir ~/.ssh --mode 0600

  # Save only the private key to a specific path
  jtsekret dump ssh-keys --key id_rsa --output ~/.ssh/id_rsa --mode 0600

  # Print a single entry to stdout
  jtsekret dump ssh-keys --key id_rsa --output -`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: secretNameCompletion,
	RunE:              runDump,
}

func init() {
	dumpCmd.Flags().StringVar(&dumpKey, "key", "", "entry key to dump (default: all keys)")
	dumpCmd.Flags().StringVar(&dumpDir, "dir", ".", "directory to save files into")
	dumpCmd.Flags().StringVar(&dumpOutput, "output", "", "output file path; use - for stdout (requires --key)")
	dumpCmd.Flags().StringVar(&dumpMode, "mode", "0600", "file permission bits in octal (e.g. 0600, 0644)")
	dumpCmd.Flags().BoolVar(&dumpForce, "force", false, "overwrite existing files without prompt")
	dumpCmd.Flags().BoolVar(&dumpNoCache, "no-cache", false, "skip cache and fetch from backend")

	rootCmd.AddCommand(dumpCmd)
}

func runDump(cmd *cobra.Command, args []string) error {
	name := args[0]

	if dumpOutput != "" && dumpKey == "" {
		return errors.New("--output requires --key")
	}

	modeVal, err := strconv.ParseUint(dumpMode, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid --mode %q: must be an octal number like 0600", dumpMode)
	}
	fileMode := os.FileMode(modeVal)

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}

	var c cache.Cache
	if cfg.Cache.Enabled && !dumpNoCache {
		c, err = cache.NewCache(ctx, cfg.Cache)
		if err != nil {
			return fmt.Errorf("init cache: %w", err)
		}
	}

	var payload *backend.Payload

	if c != nil {
		cached, err := c.Get(ctx, name)
		if err == nil && cached != nil {
			entries := make([]backend.Entry, 0, len(cached.Entries))
			for k, v := range cached.Entries {
				entries = append(entries, backend.Entry{Key: k, Value: v})
			}
			payload = &backend.Payload{
				SecretID:   name,
				VersionID:  cached.VersionID,
				Entries:    entries,
				EntriesMap: cached.Entries,
			}
		}
	}

	if payload == nil {
		payload, err = b.GetPayload(ctx, name, "")
		_ = audit.FromError("dump", cfg.Backend.Type, name, dumpKey, err)
		if err != nil {
			return fmt.Errorf("get payload: %w", err)
		}

		if c != nil {
			entriesMap := make(map[string][]byte)
			for _, e := range payload.Entries {
				entriesMap[e.Key] = e.Value
			}
			ttl := time.Duration(cfg.Cache.TTL) * time.Second
			if cacheErr := c.Set(ctx, name, &domain.CachedPayload{
				Entries:   entriesMap,
				CachedAt:  time.Now(),
				TTL:       ttl,
				VersionID: payload.VersionID,
			}); cacheErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write cache: %v\n", cacheErr)
			}
		}
	}

	// Build the list of entries to dump
	var entries []backend.Entry
	if dumpKey != "" {
		for _, e := range payload.Entries {
			if e.Key == dumpKey {
				entries = []backend.Entry{e}
				break
			}
		}
		if len(entries) == 0 {
			return fmt.Errorf("key %q not found in secret %q", dumpKey, name)
		}
	} else {
		entries = payload.Entries
	}

	for _, e := range entries {
		if err := dumpEntry(e, fileMode); err != nil {
			return err
		}
	}

	return nil
}

func dumpEntry(e backend.Entry, mode os.FileMode) error {
	// --output - means stdout
	if dumpOutput == "-" {
		_, err := os.Stdout.Write(e.Value)
		return err
	}

	// Determine destination path
	dest := dumpOutput
	if dest == "" {
		dir := dumpDir
		if dir == "" {
			dir = "."
		}
		expanded, err := expandHome(dir)
		if err != nil {
			return fmt.Errorf("expand dir path: %w", err)
		}
		dest = filepath.Join(expanded, e.Key)
	} else {
		expanded, err := expandHome(dest)
		if err != nil {
			return fmt.Errorf("expand output path: %w", err)
		}
		dest = expanded
	}

	// Check for existing file
	if !dumpForce {
		if _, err := os.Stat(dest); err == nil {
			fmt.Fprintf(os.Stderr, "File %s already exists. Overwrite? (yes/no): ", dest)
			var response string
			_, _ = fmt.Scanln(&response)
			if response != "yes" {
				fmt.Fprintf(os.Stderr, "Skipped %s\n", dest)
				return nil
			}
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("create directory for %s: %w", dest, err)
	}

	// Write atomically via temp file in the same directory
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".jtsekret-dump-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", dest, err)
	}
	tmpName := tmp.Name()

	_, writeErr := tmp.Write(e.Value)
	closeErr := tmp.Close()
	if writeErr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", dest, writeErr)
	}
	if closeErr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file for %s: %w", dest, closeErr)
	}

	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod %s: %w", dest, err)
	}

	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename to %s: %w", dest, err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s (mode %s)\n", dest, mode)
	return nil
}

func expandHome(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
