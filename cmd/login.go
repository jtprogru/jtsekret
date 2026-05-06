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
	"os/exec"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [provider]",
	Short: "Authenticate with a backend provider",
	Long: `Run an interactive authentication flow for a backend provider.

Currently supported:
  yc    Yandex Cloud (delegates to the official ` + "`yc init`" + ` browser flow).
        After this, jtsekret with auth.type=auto picks up credentials
        automatically via ` + "`yc iam create-token`" + `.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	provider := "yc"
	if len(args) == 1 {
		provider = args[0]
	}
	switch provider {
	case "yc":
		return loginYC()
	default:
		return fmt.Errorf("unknown provider %q (supported: yc)", provider)
	}
}

func loginYC() error {
	if _, err := exec.LookPath("yc"); err != nil {
		return errors.New(
			"yc CLI not found in PATH.\n" +
				"  Install it from https://cloud.yandex.com/docs/cli/quickstart, then re-run `jtsekret login yc`.\n" +
				"  Alternatively set YC_OAUTH_TOKEN or YC_IAM_TOKEN manually.")
	}
	fmt.Fprintln(os.Stdout, "Launching `yc init` (browser-based OAuth flow). Follow the prompts in your browser…")
	c := exec.Command("yc", "init")
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("yc init: %w", err)
	}
	if out, err := exec.Command("yc", "iam", "create-token").Output(); err == nil && len(out) > 0 {
		fmt.Fprintln(os.Stdout, "OK — `yc iam create-token` produced a token. jtsekret will now auto-refresh tokens via yc.")
		return nil
	}
	fmt.Fprintln(os.Stdout, "yc init completed, but `yc iam create-token` did not return a token. Check `yc config list`.")
	return nil
}
