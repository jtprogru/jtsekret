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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jtprogru/jtsekret/internal/audit"
)

var (
	auditTail int
	auditJSON bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Inspect or manage the local audit log",
}

var auditShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the most recent audit-log entries",
	RunE:  runAuditShow,
}

var auditPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the resolved audit-log file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := audit.Path()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, p)
		return nil
	},
}

func init() {
	auditShowCmd.Flags().IntVarP(&auditTail, "tail", "n", 20, "show only the most recent N entries (0 = all)")
	auditShowCmd.Flags().BoolVar(&auditJSON, "json", false, "emit entries as JSON lines instead of table-style")
	auditCmd.AddCommand(auditShowCmd)
	auditCmd.AddCommand(auditPathCmd)
	rootCmd.AddCommand(auditCmd)
}

func runAuditShow(cmd *cobra.Command, args []string) error {
	entries, err := audit.LastN(auditTail)
	if err != nil {
		return fmt.Errorf("read audit log: %w", err)
	}
	if auditJSON {
		enc := json.NewEncoder(os.Stdout)
		for _, e := range entries {
			if err := enc.Encode(e); err != nil {
				return err
			}
		}
		return nil
	}
	for _, e := range entries {
		ts := e.Time.Local().Format(time.RFC3339) //nolint:gosmopolitan // CLI audit log is shown to a single human user; their local TZ is correct here
		key := e.Key
		if key == "" {
			key = "-"
		}
		result := e.Result
		if e.Error != "" {
			result = result + ": " + e.Error
		}
		fmt.Fprintf(os.Stdout, "%s  %-7s %-10s %-30s %-15s %s\n",
			ts, e.Action, e.Backend, e.Secret, key, result)
	}
	return nil
}
