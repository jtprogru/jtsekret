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
	"strings"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/crypto"
)

var (
	createDesc   string
	createLabels []string
	createKey    string
	createValue  string
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVar(&createDesc, "desc", "", "secret description")
	createCmd.Flags().StringArrayVar(&createLabels, "label", []string{}, "label in k=v format")
	createCmd.Flags().StringVar(&createKey, "key", "", "initial key name")
	createCmd.Flags().StringVar(&createValue, "value", "", "initial key value")

	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	lockboxCfg := map[string]interface{}{
		"folder_id": cfg.Backend.Lockbox.FolderID,
		"auth": map[string]interface{}{
			"type":                 cfg.Backend.Lockbox.Auth.Type,
			"token":                cfg.Backend.Lockbox.Auth.Token,
			"service_account_file": cfg.Backend.Lockbox.Auth.ServiceAccountFile,
		},
	}

	b, err := backend.New(cfg.Backend.Type, lockboxCfg)
	if err != nil {
		return fmt.Errorf("create backend: %w", err)
	}

	labels := make(map[string]string)
	for _, l := range createLabels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}

	var entries []backend.Entry
	if createKey != "" {
		value := createValue
		if value == "" {
			value, err = crypto.PromptPassword("Enter value: ")
			if err != nil {
				return fmt.Errorf("read value: %w", err)
			}
		}
		entries = []backend.Entry{{Key: createKey, Value: []byte(value)}}
	}

	secret, err := b.CreateSecret(ctx, name, createDesc, entries)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}

	fmt.Printf("Created secret %q (ID: %s)\n", secret.Name, secret.ID)
	return nil
}
