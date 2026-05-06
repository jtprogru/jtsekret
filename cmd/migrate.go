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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/config"
)

var (
	migrateTargetCfg string
	migrateUpdate    bool
	migrateDryRun    bool
	migrateOnly      []string
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Copy all secrets from the configured backend to a target backend",
	Long: `Reads every secret + payload from the source backend (current --config)
and writes it to the target backend defined in --target-config.

Behaviour for collisions:
  default          fail and stop on the first secret that already exists in the target
  --update         add a new version on top of the existing target secret
  --dry-run        list what would be copied without touching the target

Use --only NAME[,NAME] to migrate a subset.`,
	RunE: runMigrate,
}

func init() {
	migrateCmd.Flags().StringVar(&migrateTargetCfg, "target-config", "", "path to a jtsekret config file describing the target backend (required)")
	migrateCmd.Flags().BoolVar(&migrateUpdate, "update", false, "if a secret already exists in target, add a new version instead of failing")
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "show what would be migrated without writing")
	migrateCmd.Flags().StringSliceVar(&migrateOnly, "only", nil, "migrate only the named secrets (comma-separated)")
	_ = migrateCmd.MarkFlagRequired("target-config")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	srcCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load source config: %w", err)
	}
	tgtCfg, err := config.LoadFromFile(migrateTargetCfg)
	if err != nil {
		return fmt.Errorf("load target config: %w", err)
	}
	if srcCfg.Backend.Type == "" || tgtCfg.Backend.Type == "" {
		return errors.New("both source and target configs must declare backend.type")
	}

	src, err := buildBackend(srcCfg)
	if err != nil {
		return fmt.Errorf("source backend: %w", err)
	}
	tgt, err := buildBackend(tgtCfg)
	if err != nil {
		return fmt.Errorf("target backend: %w", err)
	}

	stats, err := MigrateBackends(ctx, src, tgt, MigrateOptions{
		Update: migrateUpdate,
		DryRun: migrateDryRun,
		Only:   migrateOnly,
		Out:    os.Stdout,
	})
	if err != nil {
		return err
	}

	if migrateDryRun {
		fmt.Fprintf(os.Stdout, "dry run: %d source secrets considered\n", stats.Total)
		return nil
	}
	fmt.Fprintf(os.Stdout, "done: %d created, %d updated, %d skipped\n", stats.Created, stats.Updated, stats.Skipped)
	return nil
}

type MigrateOptions struct {
	Update bool
	DryRun bool
	Only   []string
	Out    interface {
		Write(p []byte) (n int, err error)
	}
}

type MigrateStats struct {
	Total, Created, Updated, Skipped int
}

// MigrateBackends copies every secret from src to tgt, honouring opts.
// Pure orchestration logic — no config or cobra coupling, easy to test.
func MigrateBackends(ctx context.Context, src, tgt backend.Backend, opts MigrateOptions) (MigrateStats, error) {
	var stats MigrateStats
	wantedSet := map[string]bool{}
	for _, n := range opts.Only {
		n = strings.TrimSpace(n)
		if n != "" {
			wantedSet[n] = true
		}
	}

	secrets, err := src.ListSecrets(ctx)
	if err != nil {
		return stats, fmt.Errorf("list source secrets: %w", err)
	}
	stats.Total = len(secrets)

	tgtIndex := map[string]struct{}{}
	tgtList, err := tgt.ListSecrets(ctx)
	if err != nil {
		return stats, fmt.Errorf("list target secrets: %w", err)
	}
	for _, s := range tgtList {
		tgtIndex[s.Name] = struct{}{}
	}

	for _, s := range secrets {
		if len(wantedSet) > 0 && !wantedSet[s.Name] {
			stats.Skipped++
			continue
		}
		payload, err := src.GetPayload(ctx, s.Name, "")
		if err != nil {
			return stats, fmt.Errorf("read source %q: %w", s.Name, err)
		}
		entries := toBackendEntries(payload.Entries)
		_, exists := tgtIndex[s.Name]

		switch {
		case opts.DryRun && exists:
			fmt.Fprintf(opts.Out, "DRY-RUN  %s  [exists, would %s]\n",
				s.Name, ifThen(opts.Update, "add version", "fail"))
		case opts.DryRun:
			fmt.Fprintf(opts.Out, "DRY-RUN  %s  [create]\n", s.Name)
		case exists && !opts.Update:
			return stats, fmt.Errorf("secret %q already exists in target (pass --update to overwrite)", s.Name)
		case exists:
			if err := tgt.AddVersion(ctx, s.Name, entries); err != nil {
				return stats, fmt.Errorf("update target %q: %w", s.Name, err)
			}
			fmt.Fprintf(opts.Out, "updated  %s\n", s.Name)
			stats.Updated++
		default:
			if _, err := tgt.CreateSecret(ctx, s.Name, s.Description, entries); err != nil {
				return stats, fmt.Errorf("create target %q: %w", s.Name, err)
			}
			fmt.Fprintf(opts.Out, "created  %s\n", s.Name)
			stats.Created++
		}
	}
	return stats, nil
}

func toBackendEntries(in []backend.Entry) []backend.Entry {
	// AddVersion/CreateSecret want []backend.Entry — payload already returns
	// that, but defensive copy keeps the source backend's slice from being
	// mutated by a target that holds onto it.
	out := make([]backend.Entry, len(in))
	for i, e := range in {
		v := make([]byte, len(e.Value))
		copy(v, e.Value)
		out[i] = backend.Entry{Key: e.Key, Value: v}
	}
	return out
}

func ifThen(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
