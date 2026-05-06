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
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// LogLevel is the dynamic level shared with the slog handler set up in
// main.main(). PersistentPreRunE flips it to Debug when --debug is passed
// or to whatever log.level says in the config file.
var LogLevel slog.LevelVar

var (
	cfgFile    string
	outputMode string
	noCache    bool
	debugMode  bool
)

var rootCmd = &cobra.Command{
	Use:   "jtsekret",
	Short: "CLI utility for secure secrets management",
	Long: `jtsekret is a CLI tool for centralized and secure management of personal secrets
(passwords, OAuth tokens, API keys, bot tokens). It abstracts the storage backend
through a unified interface, implements a local encrypted cache, and allows
passing retrieved secrets to other processes via Unix pipe.

Supported backends:
  - Yandex Cloud Lockbox

Examples:
  jtsekret get my-api-token --key token | curl -H "Authorization: Bearer $(cat)" https://api.example.com
  jtsekret list
  jtsekret cache status`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if noCache {
			viper.Set("cache.enabled", false)
		}
		applyLogLevel()
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.jtsekret.yaml)")
	rootCmd.PersistentFlags().StringVar(&outputMode, "output", "plain", "output format: plain|table|json")
	rootCmd.PersistentFlags().BoolVar(&noCache, "no-cache", false, "disable local cache")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug logging")

	viper.SetEnvPrefix("JTSEKRET")
	viper.BindPFlag("output.format", rootCmd.PersistentFlags().Lookup("output"))
}

func initConfig() {
	viper.SetConfigType("yaml")

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(filepath.Join(home, ".config", "jtsekret"))
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigName("jtsekret")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		// Apply level here too so the config-file path is visible under --debug
		// — initConfig runs before PersistentPreRunE.
		applyLogLevel()
		slog.Debug("using config file", slog.String("path", viper.ConfigFileUsed()))
	}
}

func applyLogLevel() {
	if debugMode {
		LogLevel.Set(slog.LevelDebug)
		return
	}
	switch viper.GetString("log.level") {
	case "debug":
		LogLevel.Set(slog.LevelDebug)
	case "info":
		LogLevel.Set(slog.LevelInfo)
	case "error":
		LogLevel.Set(slog.LevelError)
	default:
		LogLevel.Set(slog.LevelWarn)
	}
}
