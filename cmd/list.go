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
	"github.com/spf13/viper"

	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/domain"
	"github.com/jtprogru/jtsekret/internal/output"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all secrets",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
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

	outputFormat := output.OutputFormat(viper.GetString("output.format"))
	if outputFormat == output.FormatAuto {
		outputFormat = output.DetectFormat()
	}
	out := output.NewOutputter(outputFormat)

	domSecrets := make([]domain.Secret, len(secrets))
	for i, s := range secrets {
		domSecrets[i] = domain.Secret{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Labels:      s.Labels,
			EntryKeys:   s.EntryKeys,
		}
	}

	return out.PrintSecretList(os.Stdout, domSecrets)
}
