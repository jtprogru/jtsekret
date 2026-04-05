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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/cache"
	"github.com/jtprogru/jtsekret/internal/config"
	"github.com/jtprogru/jtsekret/internal/domain"
	"github.com/jtprogru/jtsekret/internal/output"
)

var (
	getKey       string
	getVersion   string
	getOutputRaw bool
	getNoCache   bool
)

var getCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a secret or specific key",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func init() {
	getCmd.Flags().StringVar(&getKey, "key", "", "specific key to retrieve")
	getCmd.Flags().StringVar(&getVersion, "version", "", "specific version ID")
	getCmd.Flags().BoolVar(&getOutputRaw, "raw", false, "output raw value without key name")
	getCmd.Flags().BoolVar(&getNoCache, "no-cache", false, "skip cache and fetch from backend")

	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	lockboxCfg := map[string]interface{}{
		"folder_id": cfg.Backend.Lockbox.FolderID,
		"endpoint":  cfg.Backend.Lockbox.Endpoint,
		"auth": map[string]string{
			"type":  cfg.Backend.Lockbox.Auth.Type,
			"token": cfg.Backend.Lockbox.Auth.Token,
		},
	}

	b, err := backend.New(cfg.Backend.Type, lockboxCfg)
	if err != nil {
		return fmt.Errorf("create backend: %w", err)
	}

	var c cache.Cache
	if cfg.Cache.Enabled && !getNoCache {
		c, err = cache.NewCache(ctx, cfg.Cache)
		if err != nil {
			return fmt.Errorf("init cache: %w", err)
		}
	}

	outputFormat := output.OutputFormat(viper.GetString("output.format"))
	if outputFormat == output.FormatAuto {
		outputFormat = output.DetectFormat()
	}
	out := output.NewOutputter(outputFormat)

	var payload *backend.Payload

	if c != nil {
		cached, err := c.Get(ctx, name)
		if err == nil && cached != nil {
			entries := make([]backend.Entry, 0, len(cached.Entries))
			for k, v := range cached.Entries {
				entries = append(entries, backend.Entry{Key: k, Value: v})
			}
			payload = &backend.Payload{
				SecretID:   name,
				VersionID:  cached.VersionID,
				Entries:    entries,
				EntriesMap: cached.Entries,
			}
		}
	}

	if payload == nil {
		payload, err = b.GetPayload(ctx, name, getVersion)
		if err != nil {
			return fmt.Errorf("get payload: %w", err)
		}

		if c != nil {
			entriesMap := make(map[string][]byte)
			for _, e := range payload.Entries {
				entriesMap[e.Key] = e.Value
			}
			ttl := time.Duration(cfg.Cache.TTL) * time.Second
			if cacheErr := c.Set(ctx, name, &domain.CachedPayload{
				Entries:   entriesMap,
				CachedAt:  time.Now(),
				TTL:       ttl,
				VersionID: payload.VersionID,
			}); cacheErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write cache: %v\n", cacheErr)
			}
		}
	}

	if getKey != "" {
		for _, e := range payload.Entries {
			if e.Key == getKey {
				if getOutputRaw {
					os.Stdout.Write(e.Value)
				} else {
					out.PrintEntry(os.Stdout, e.Key, e.Value)
				}
				return nil
			}
		}
		return fmt.Errorf("key %q not found", getKey)
	}

	if getOutputRaw {
		for _, e := range payload.Entries {
			os.Stdout.Write(e.Value)
			os.Stdout.Write([]byte{'\n'})
		}
		return nil
	}

	domPayload := backend.PayloadToDomain(payload)
	return out.PrintPayload(os.Stdout, domPayload)
}
