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
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/domain"
	"github.com/jtprogru/jtsekret/internal/output"
)

var searchIncludeKeys bool

var searchCmd = &cobra.Command{
	Use:   "search <pattern>",
	Short: "Find secrets by name (case-insensitive substring match)",
	Long: `Print every secret whose name contains <pattern> (case-insensitive).
With --include-keys, also matches against entry keys (the key names within
each secret payload). Values are NEVER searched — that would require
fetching every payload from the backend.`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().BoolVar(&searchIncludeKeys, "include-keys", false, "also match against entry keys, not just secret names")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	pattern := strings.ToLower(args[0])
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}
	secrets, err := b.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("list secrets: %w", err)
	}

	matched := make([]domain.Secret, 0, len(secrets))
	for _, s := range secrets {
		hit := strings.Contains(strings.ToLower(s.Name), pattern)
		if !hit && searchIncludeKeys {
			for _, k := range s.EntryKeys {
				if strings.Contains(strings.ToLower(k), pattern) {
					hit = true
					break
				}
			}
		}
		if hit {
			matched = append(matched, domain.Secret{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
				Labels:      s.Labels,
				EntryKeys:   s.EntryKeys,
			})
		}
	}

	outputFormat := output.Format(viper.GetString("output.format"))
	if outputFormat == output.FormatAuto {
		outputFormat = output.DetectFormat()
	}
	out := output.NewOutputter(outputFormat)
	return out.PrintSecretList(os.Stdout, matched)
}
