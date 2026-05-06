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
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/audit"
	"github.com/jtprogru/jtsekret/internal/config"
)

var (
	execSecret  string
	execKey     string
	execEnvVar  string
	execStdin   bool
	execNoCache bool
)

var execCmd = &cobra.Command{
	Use:   "exec --secret <name> --key <key> -- <command> [args]",
	Short: "Run a process with a secret",
	Long: `Run a command with a secret passed via stdin or environment variable.
This is the safest way to pass secrets to other processes as the value
never appears in shell history.`,
	DisableFlagsInUseLine: true,
	Args:                  cobra.ArbitraryArgs,
	RunE:                  runExec,
}

func init() {
	execCmd.Flags().StringVar(&execSecret, "secret", "", "secret name (required)")
	execCmd.Flags().StringVar(&execKey, "key", "", "key to retrieve (required)")
	execCmd.Flags().StringVar(&execEnvVar, "env", "", "environment variable name")
	execCmd.Flags().BoolVar(&execStdin, "stdin", false, "pass secret via stdin")
	execCmd.Flags().BoolVar(&execNoCache, "no-cache", false, "skip cache")

	_ = execCmd.MarkFlagRequired("secret")
	_ = execCmd.MarkFlagRequired("key")
	_ = execCmd.RegisterFlagCompletionFunc("secret", secretNameCompletion)

	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("command required")
	}

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}

	payload, err := b.GetPayload(ctx, execSecret, "")
	_ = audit.FromError("exec", cfg.Backend.Type, execSecret, execKey, err)
	if err != nil {
		return fmt.Errorf("get payload: %w", err)
	}

	var secretValue string
	for _, e := range payload.Entries {
		if e.Key == execKey {
			secretValue = string(e.Value)
			break
		}
	}
	if secretValue == "" {
		return fmt.Errorf("key %q not found in secret", execKey)
	}

	command := args[0]
	commandArgs := args[1:]

	c := exec.CommandContext(ctx, command, commandArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if execStdin {
		c.Stdin = strings.NewReader(secretValue)
	}

	if execEnvVar != "" {
		env := os.Environ()
		env = append(env, fmt.Sprintf("%s=%s", execEnvVar, secretValue))
		c.Env = env
	}

	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("execute command: %w", err)
	}

	return nil
}
