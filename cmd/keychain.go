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
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/crypto"
	"github.com/jtprogru/jtsekret/internal/keychain"
)

var keychainCmd = &cobra.Command{
	Use:   "keychain",
	Short: "Store master passwords in the macOS Keychain (avoids JTSEKRET_*_MASTER_PASSWORD env vars)",
	Long: `Manage jtsekret master passwords stored in the macOS Keychain.
Three slots are recognised: cache, github, file (one per backend that
holds local ciphertext under a user-supplied master password).

Resolution chain at runtime, per slot:
  1. JTSEKRET_<SLOT>_MASTER_PASSWORD env var
  2. macOS Keychain entry (Service=jtsekret, Account=<slot>)

Non-darwin platforms silently skip step 2; on those use env vars.`,
}

var keychainSetCmd = &cobra.Command{
	Use:               "set <slot>",
	Short:             "Store a master password in the keychain (prompts twice for confirmation)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: keychainSlotCompletion,
	RunE:              runKeychainSet,
}

var keychainGetCmd = &cobra.Command{
	Use:               "get <slot>",
	Short:             "Print the password stored in the keychain (Keychain may prompt for permission)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: keychainSlotCompletion,
	RunE:              runKeychainGet,
}

var keychainUnsetCmd = &cobra.Command{
	Use:               "unset <slot>",
	Short:             "Remove a slot from the keychain",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: keychainSlotCompletion,
	RunE:              runKeychainUnset,
}

var keychainListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show which jtsekret slots currently exist in the keychain",
	RunE:  runKeychainList,
}

func init() {
	keychainCmd.AddCommand(keychainSetCmd, keychainGetCmd, keychainUnsetCmd, keychainListCmd)
	rootCmd.AddCommand(keychainCmd)
}

func keychainSlotCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return keychain.KnownSlots, cobra.ShellCompDirectiveNoFileComp
}

func runKeychainSet(cmd *cobra.Command, args []string) error {
	slot := args[0]
	if !keychain.IsKnownSlot(slot) {
		return fmt.Errorf("unknown slot %q (known: %s)", slot, strings.Join(keychain.KnownSlots, ", "))
	}
	pwd, err := crypto.PromptPassword(fmt.Sprintf("Master password for %q: ", slot))
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if pwd == "" {
		return errors.New("password is empty")
	}
	confirm, err := crypto.PromptPassword("Confirm: ")
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if confirm != pwd {
		return errors.New("passwords do not match")
	}
	if err := keychain.Set(cmd.Context(), slot, pwd); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Stored master password for %q in keychain.\n", slot)
	return nil
}

func runKeychainGet(cmd *cobra.Command, args []string) error {
	slot := args[0]
	if !keychain.IsKnownSlot(slot) {
		return fmt.Errorf("unknown slot %q (known: %s)", slot, strings.Join(keychain.KnownSlots, ", "))
	}
	v, err := keychain.Get(cmd.Context(), slot)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, v)
	return nil
}

func runKeychainUnset(cmd *cobra.Command, args []string) error {
	slot := args[0]
	if !keychain.IsKnownSlot(slot) {
		return fmt.Errorf("unknown slot %q (known: %s)", slot, strings.Join(keychain.KnownSlots, ", "))
	}
	if err := keychain.Delete(cmd.Context(), slot); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Removed %q from keychain.\n", slot)
	return nil
}

func runKeychainList(cmd *cobra.Command, args []string) error {
	if !keychain.Available() {
		fmt.Fprintln(os.Stderr, "macOS Keychain is not available on this platform — slots can only be set via JTSEKRET_*_MASTER_PASSWORD env vars.")
		return nil
	}
	slots, err := keychain.List(cmd.Context())
	if err != nil {
		return err
	}
	if len(slots) == 0 {
		fmt.Fprintln(os.Stdout, "(no jtsekret entries in keychain)")
		return nil
	}
	for _, s := range slots {
		fmt.Fprintln(os.Stdout, s)
	}
	return nil
}
