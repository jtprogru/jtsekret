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
	"fmt"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/config"
)

// buildBackend resolves the configured backend and constructs an instance.
// It returns the constructed backend and the original cfg.Backend.Type.
func buildBackend(cfg *config.Config) (backend.Backend, error) {
	rawCfg, err := backendConfigMap(cfg)
	if err != nil {
		return nil, err
	}
	b, err := backend.New(cfg.Backend.Type, rawCfg)
	if err != nil {
		return nil, fmt.Errorf("create backend %q: %w", cfg.Backend.Type, err)
	}
	return b, nil
}

func backendConfigMap(cfg *config.Config) (map[string]interface{}, error) {
	switch cfg.Backend.Type {
	case "lockbox":
		return map[string]interface{}{
			"folder_id": cfg.Backend.Lockbox.FolderID,
			"auth": map[string]interface{}{
				"type":                 cfg.Backend.Lockbox.Auth.Type,
				"token":                cfg.Backend.Lockbox.Auth.Token,
				"service_account_file": cfg.Backend.Lockbox.Auth.ServiceAccountFile,
			},
		}, nil
	case "github":
		return map[string]interface{}{
			"repo":            cfg.Backend.Github.Repo,
			"branch":          cfg.Backend.Github.Branch,
			"local_path":      cfg.Backend.Github.GetLocalPath(),
			"auto_pull":       cfg.Backend.Github.AutoPull,
			"auto_push":       cfg.Backend.Github.AutoPush,
			"master_password": cfg.Backend.Github.MasterPassword,
			"auth": map[string]interface{}{
				"type":            cfg.Backend.Github.Auth.Type,
				"token":           cfg.Backend.Github.Auth.Token,
				"ssh_key_path":     cfg.Backend.Github.Auth.SSHKeyPath,
				"ssh_key_password": cfg.Backend.Github.Auth.SSHKeyPassword,
			},
		}, nil
	case "file":
		return map[string]interface{}{
			"path":            cfg.Backend.File.GetPath(),
			"master_password": cfg.Backend.File.MasterPassword,
		}, nil
	case "mock":
		return map[string]interface{}{}, nil
	case "":
		return nil, fmt.Errorf("backend.type is not set in config")
	default:
		return nil, fmt.Errorf("unsupported backend type: %s", cfg.Backend.Type)
	}
}
