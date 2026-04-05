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
	"errors"
	"fmt"
	"os"
)

var (
	ErrMissingBackendType    = errors.New("backend type is required")
	ErrMissingFolderID       = errors.New("lockbox folder_id is required")
	ErrMissingCachePath      = errors.New("cache path is required when cache is enabled")
	ErrMissingMasterPassword = errors.New("cache master password is required when cache is enabled")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s - %s", e.Field, e.Message)
}

func Validate(cfg *Config) error {
	if cfg.Backend.Type == "" {
		return &ValidationError{Field: "backend.type", Message: ErrMissingBackendType.Error()}
	}

	if cfg.Backend.Type == "lockbox" {
		if cfg.Backend.Lockbox.FolderID == "" {
			return &ValidationError{Field: "backend.lockbox.folder_id", Message: ErrMissingFolderID.Error()}
		}
		if cfg.Backend.Lockbox.Auth.Type == "" {
			cfg.Backend.Lockbox.Auth.Type = "oauth"
		}
	}

	if cfg.Cache.Enabled {
		if cfg.Cache.Path == "" {
			return &ValidationError{Field: "cache.path", Message: ErrMissingCachePath.Error()}
		}
		if cfg.Cache.MasterPassword == "" {
			cfg.Cache.MasterPassword = os.Getenv("JTSEKRET_CACHE_MASTER_PASSWORD")
			if cfg.Cache.MasterPassword == "" {
				return &ValidationError{Field: "cache.master_password", Message: ErrMissingMasterPassword.Error()}
			}
		}
	}

	return nil
}

func ValidateBackendConnection(cfg *Config) error {
	return nil
}
