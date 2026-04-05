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
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Backend BackendConfig `mapstructure:"backend"`
	Cache   CacheConfig   `mapstructure:"cache"`
	Output  OutputConfig  `mapstructure:"output"`
	Log     LogConfig     `mapstructure:"log"`
}

type BackendConfig struct {
	Type    string         `mapstructure:"type"`
	Lockbox LockboxConfig  `mapstructure:"lockbox"`
	Custom  map[string]any `mapstructure:",remain"`
}

type LockboxConfig struct {
	FolderID string      `mapstructure:"folder_id"`
	Auth     LockboxAuth `mapstructure:"auth"`
}

type LockboxAuth struct {
	Type               string `mapstructure:"type"`
	Token              string `mapstructure:"token"`
	ServiceAccountFile string `mapstructure:"service_account_file"`
}

type CacheConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	TTL            int    `mapstructure:"ttl"`
	Path           string `mapstructure:"path"`
	MasterPassword string `mapstructure:"-"`
}

type OutputConfig struct {
	Format       string `mapstructure:"format"`
	KeySeparator string `mapstructure:"key_separator"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func Load() (*Config, error) {
	v := viper.GetViper()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Cache.MasterPassword = os.Getenv("JTSEKRET_CACHE_MASTER_PASSWORD")

	return cfg, nil
}

func (c *Config) GetCachePath() string {
	if strings.HasPrefix(c.Cache.Path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return c.Cache.Path
		}
		return filepath.Join(home, strings.TrimPrefix(c.Cache.Path, "~"))
	}
	return c.Cache.Path
}

func (c *OutputConfig) GetFormat() string {
	switch c.Format {
	case "table", "json":
		return c.Format
	default:
		return "plain"
	}
}

func (c *LogConfig) GetLevel() string {
	switch c.Level {
	case "debug", "info", "warn", "error":
		return c.Level
	default:
		return "warn"
	}
}
