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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/config"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize the configured backend with its remote (git pull/push)",
	Long: `Forces an explicit pull-then-push cycle for backends that have a
local working copy. Useful when auto_pull/auto_push are both disabled.

For backends that always talk to an authoritative remote (lockbox, vault)
the command is a no-op.`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}
	syncer, ok := b.(backend.Syncer)
	if !ok {
		fmt.Fprintf(os.Stdout, "backend %q has no explicit sync step (always remote-first)\n", cfg.Backend.Type)
		return nil
	}
	if err := syncer.Sync(context.Background()); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	fmt.Fprintln(os.Stdout, "sync ok")
	return nil
}
