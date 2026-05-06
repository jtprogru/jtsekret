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

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/crypto"
)

var rotateMasterCmd = &cobra.Command{
	Use:   "rotate-master",
	Short: "Re-encrypt all secrets in the local-master backend under a new password",
	Long: `For backends that hold ciphertext at-rest under a user-supplied master
password (github, file), decrypts every secret with the current password
and re-writes it with a fresh per-secret salt under the new password.

Backends that delegate keys to a cloud KMS (lockbox, vault) reject this
command — there's no local master to rotate.

The current password is taken from JTSEKRET_<BACKEND>_MASTER_PASSWORD
(or JTSEKRET_CACHE_MASTER_PASSWORD as a fallback). The new password is
prompted interactively (twice for confirmation).

NOTE: this command does NOT update the cache. After a successful rotation
run with the new master password set in your environment.`,
	RunE: runRotateMaster,
}

func init() {
	rootCmd.AddCommand(rotateMasterCmd)
}

func runRotateMaster(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}
	rotator, ok := b.(backend.MasterPasswordRotator)
	if !ok {
		return fmt.Errorf("backend %q does not have a local master password to rotate "+
			"(only github and file backends do)", cfg.Backend.Type)
	}

	newPass, err := crypto.PromptPassword("New master password: ")
	if err != nil {
		return fmt.Errorf("read new password: %w", err)
	}
	if newPass == "" {
		return errors.New("new password is empty")
	}
	confirm, err := crypto.PromptPassword("Confirm new master password: ")
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if confirm != newPass {
		return errors.New("passwords do not match")
	}

	ctx := context.Background()
	fmt.Fprintln(os.Stderr, "Rotating master password — re-encrypting every secret. This may take a moment…")
	if err := rotator.RotateMasterPassword(ctx, newPass); err != nil {
		return fmt.Errorf("rotate master password: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Master password rotated. Update your environment to use the new password before the next jtsekret invocation.")
	return nil
}
