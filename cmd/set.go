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

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/config"
)

var setCmd = &cobra.Command{
	Use:   "set <name> <key> <value>",
	Short: "Add or update a key in a secret",
	Args:  cobra.ExactArgs(3),
	RunE:  runSet,
}

func init() {
	rootCmd.AddCommand(setCmd)
}

func runSet(cmd *cobra.Command, args []string) error {
	name := args[0]
	key := args[1]
	value := args[2]
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}

	currentPayload, err := b.GetPayload(ctx, name, "")
	if err != nil {
		return fmt.Errorf("get current payload: %w", err)
	}

	entries := make([]backend.Entry, len(currentPayload.Entries))
	copy(entries, currentPayload.Entries)

	found := false
	for i, e := range entries {
		if e.Key == key {
			entries[i].Value = []byte(value)
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, backend.Entry{Key: key, Value: []byte(value)})
	}

	err = b.AddVersion(ctx, name, entries)
	if err != nil {
		return fmt.Errorf("add version: %w", err)
	}

	fmt.Printf("Updated key %q in secret %q\n", key, name)
	return nil
}
