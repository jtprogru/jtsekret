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

// Public sentinel errors. The CLI matches on these to produce
// actionable error messages without parsing free-form strings.
var (
	ErrMissingBackendType    = errors.New("backend type is required")
	ErrMissingFolderID       = errors.New("lockbox folder_id is required")
	ErrMissingCachePath      = errors.New("cache path is required when cache is enabled")
	ErrMissingMasterPassword = errors.New("cache master password is required when cache is enabled")

	// Per-backend sentinels added in v1.0.0: each backend that ships in
	// the binary now declares its own minimum required fields so
	// `jtsekret config validate` catches typos before any network round-trip.
	ErrUnknownBackendType   = errors.New("unsupported backend type")
	ErrMissingGithubRepo    = errors.New("github backend requires repo (owner/name, full URL, or file://...)")
	ErrMissingGithubMaster  = errors.New("github backend requires a master password (JTSEKRET_GITHUB_MASTER_PASSWORD or fallback)")
	ErrMissingFileMaster    = errors.New("file backend requires a master password (JTSEKRET_FILE_MASTER_PASSWORD or fallback)")
	ErrMissingVaultAddress  = errors.New("vault backend requires address (config or VAULT_ADDR)")
	ErrUnknownVaultAuthType = errors.New("vault backend has unsupported auth.type (token | approle | userpass)")
	ErrUnknownGithubAuthType = errors.New("github backend has unsupported auth.type (token | ssh | none)")
)

// SupportedBackendTypes is the canonical list, used by Validate and by
// the CLI helpers that render error suggestions. Append-only contract
// for v1.0.0+.
var SupportedBackendTypes = []string{"github", "lockbox", "vault", "file", "mock"}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s - %s", e.Field, e.Message)
}

// Validate enforces the v1.0.0 config-schema contract. It accepts the
// configured backend type plus the fields required by that backend, and
// rejects anything else with a ValidationError pointing at the offending
// field path.
func Validate(cfg *Config) error {
	if cfg.Backend.Type == "" {
		return &ValidationError{Field: "backend.type", Message: ErrMissingBackendType.Error()}
	}
	if !isSupportedBackendType(cfg.Backend.Type) {
		return &ValidationError{
			Field: "backend.type",
			Message: fmt.Sprintf("%s: %q (supported: %v)",
				ErrUnknownBackendType.Error(), cfg.Backend.Type, SupportedBackendTypes),
		}
	}

	switch cfg.Backend.Type {
	case "lockbox":
		if cfg.Backend.Lockbox.FolderID == "" {
			return &ValidationError{Field: "backend.lockbox.folder_id", Message: ErrMissingFolderID.Error()}
		}
		if cfg.Backend.Lockbox.Auth.Type == "" {
			cfg.Backend.Lockbox.Auth.Type = "auto"
		}
	case "github":
		if cfg.Backend.Github.Repo == "" {
			return &ValidationError{Field: "backend.github.repo", Message: ErrMissingGithubRepo.Error()}
		}
		if cfg.Backend.Github.MasterPassword == "" {
			return &ValidationError{Field: "backend.github.master_password", Message: ErrMissingGithubMaster.Error()}
		}
		if cfg.Backend.Github.Auth.Type != "" {
			switch cfg.Backend.Github.Auth.Type {
			case "token", "ssh", "none":
			default:
				return &ValidationError{Field: "backend.github.auth.type", Message: ErrUnknownGithubAuthType.Error()}
			}
		}
	case "file":
		if cfg.Backend.File.MasterPassword == "" {
			return &ValidationError{Field: "backend.file.master_password", Message: ErrMissingFileMaster.Error()}
		}
	case "vault":
		if cfg.Backend.Vault.Address == "" {
			return &ValidationError{Field: "backend.vault.address", Message: ErrMissingVaultAddress.Error()}
		}
		if cfg.Backend.Vault.Auth.Type != "" {
			switch cfg.Backend.Vault.Auth.Type {
			case "token", "approle", "userpass":
			default:
				return &ValidationError{Field: "backend.vault.auth.type", Message: ErrUnknownVaultAuthType.Error()}
			}
		}
	case "mock":
		// no required fields
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

func isSupportedBackendType(s string) bool {
	for _, b := range SupportedBackendTypes {
		if b == s {
			return true
		}
	}
	return false
}

// ValidateBackendConnection is a placeholder for an optional liveness
// probe (e.g. ping the configured backend). v1.0.0 ships with a stub —
// `config health` already exercises the real connection path.
func ValidateBackendConnection(_ *Config) error {
	return nil
}
