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
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jtprogru/jtsekret/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  "Commands to manage jtsekret configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create initial configuration file",
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	RunE:  runConfigValidate,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)

	rootCmd.AddCommand(configCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "jtsekret")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "jtsekret.yaml")

	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(os.Stderr, "Config file already exists: %s\n", configPath)
		return nil
	}

	exampleConfig := `backend:
  # github (default, personal-first), lockbox, mock
  type: github

  # Personal-first storage: your private GitHub repo, AES-256-GCM at rest.
  github:
    repo: "owner/my-secrets"     # owner/repo, full URL, or file://
    branch: main
    local_path: "~/.cache/jtsekret/repo"
    auto_pull: true
    auto_push: true
    auth:
      type: token                # token | ssh | none
      token: ""                  # prefer JTSEKRET_GITHUB_TOKEN env var

  # Yandex Cloud Lockbox.
  #   YC_OAUTH_TOKEN  long-lived (~1y) Yandex Passport token from oauth.yandex.ru
  #   YC_IAM_TOKEN    short-lived (~12h) IAM token from yc iam create-token
  #   These are NOT interchangeable.
  # lockbox:
  #   folder_id: ""
  #   auth:
  #     # auto: explicit token -> YC_IAM_TOKEN -> YC_OAUTH_TOKEN
  #     #       -> SA key file -> "yc iam create-token" (run "jtsekret login yc" once).
  #     type: auto

cache:
  enabled: true
  ttl: 3600
  path: "~/.cache/jtsekret/cache.enc"

output:
  format: plain
  key_separator: "\n"

log:
  level: warn
`

	if err := os.WriteFile(configPath, []byte(exampleConfig), 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	fmt.Printf("Created config file: %s\n", configPath)
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	if !viper.IsSet("backend") {
		fmt.Fprintln(os.Stderr, "No configuration loaded")
		return nil
	}

	fmt.Fprintf(os.Stdout, "Config file: %s\n", viper.ConfigFileUsed())
	fmt.Fprintf(os.Stdout, "\nBackend:\n")
	fmt.Fprintf(os.Stdout, "  Type: %s\n", viper.GetString("backend.type"))
	if viper.IsSet("backend.lockbox.folder_id") {
		fmt.Fprintf(os.Stdout, "  Folder ID: %s\n", viper.GetString("backend.lockbox.folder_id"))
	}

	fmt.Fprintf(os.Stdout, "\nCache:\n")
	fmt.Fprintf(os.Stdout, "  Enabled: %v\n", viper.GetBool("cache.enabled"))
	fmt.Fprintf(os.Stdout, "  TTL: %d\n", viper.GetInt("cache.ttl"))

	fmt.Fprintf(os.Stdout, "\nOutput:\n")
	fmt.Fprintf(os.Stdout, "  Format: %s\n", viper.GetString("output.format"))

	fmt.Fprintf(os.Stdout, "\nLog:\n")
	fmt.Fprintf(os.Stdout, "  Level: %s\n", viper.GetString("log.level"))

	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	if !viper.IsSet("backend") {
		return fmt.Errorf("no backend configuration found")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Fprintln(os.Stdout, "Configuration is valid")
	return nil
}

var configHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check backend connectivity",
	RunE:  runConfigHealth,
}

func init() {
	configCmd.AddCommand(configHealthCmd)
}

func runConfigHealth(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	b, err := buildBackend(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secrets, err := b.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("backend health check failed: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Backend is healthy. Found %d secrets.\n", len(secrets))
	return nil
}
