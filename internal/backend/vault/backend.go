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

// Package vault implements a backend backed by HashiCorp Vault's KV v2
// secret engine. Secrets map to <mount>/data/<prefix>/<name> with their
// entries materialised as `data` keys. Vault handles versioning natively,
// so AddVersion just calls Put again.
//
// Values are stored as JSON strings. UTF-8 is preserved as-is; any other
// byte stream rejected with a clear error pointing the user at the
// github / file backends, which support binary natively. This keeps the
// on-Vault representation human-friendly in the UI and avoids a bespoke
// out-of-band binary marker schema.
package vault

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/jtprogru/jtsekret/internal/backend"
)

const defaultMount = "secret"

type Backend struct {
	client *vaultapi.Client
	mount  string
	prefix string
}

func init() {
	backend.Register("vault", New)
}

func New(cfg map[string]interface{}) (backend.Backend, error) {
	address, _ := cfg["address"].(string)
	if address == "" {
		address = os.Getenv("VAULT_ADDR")
	}
	if address == "" {
		return nil, errors.New("vault backend: address is required (config backend.vault.address or VAULT_ADDR)")
	}

	mount, _ := cfg["mount"].(string)
	if mount == "" {
		mount = defaultMount
	}
	prefix, _ := cfg["prefix"].(string)
	prefix = strings.Trim(prefix, "/")

	httpClient, err := buildHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	clientCfg := vaultapi.DefaultConfig()
	clientCfg.Address = address
	clientCfg.HttpClient = httpClient
	client, err := vaultapi.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("vault client: %w", err)
	}

	if err := authenticate(client, cfg); err != nil {
		return nil, err
	}

	return &Backend{client: client, mount: mount, prefix: prefix}, nil
}

func buildHTTPClient(cfg map[string]interface{}) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	rawTLS, _ := cfg["tls"].(map[string]interface{})
	if rawTLS != nil {
		if v, ok := rawTLS["insecure"].(bool); ok && v {
			tlsCfg.InsecureSkipVerify = true
		}
		if caPath, _ := rawTLS["ca_cert"].(string); caPath != "" {
			data, err := os.ReadFile(caPath)
			if err != nil {
				return nil, fmt.Errorf("vault tls: read ca_cert: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(data) {
				return nil, fmt.Errorf("vault tls: ca_cert %q has no valid PEM certs", caPath)
			}
			tlsCfg.RootCAs = pool
		}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}, nil
}

// authenticate resolves a Vault token from cfg["auth"] and installs it
// on the client. Three auth methods are supported, all suitable for
// personal-scale use:
//
//   - token   : env VAULT_TOKEN or auth.token (no extra round-trip).
//   - approle : POST auth/<path>/login with role_id/secret_id.
//   - userpass: POST auth/<path>/login/<username> with password.
func authenticate(client *vaultapi.Client, cfg map[string]interface{}) error {
	rawAuth, _ := cfg["auth"].(map[string]interface{})
	if rawAuth == nil {
		rawAuth = map[string]interface{}{}
	}
	authType, _ := rawAuth["type"].(string)
	if authType == "" {
		authType = "token"
	}
	switch authType {
	case "token":
		token, _ := rawAuth["token"].(string)
		if token == "" {
			token = os.Getenv("VAULT_TOKEN")
		}
		if token == "" {
			return errors.New("vault backend: auth.type=token requires auth.token or VAULT_TOKEN")
		}
		client.SetToken(token)
		return nil
	case "approle":
		roleID, _ := rawAuth["role_id"].(string)
		secretID, _ := rawAuth["secret_id"].(string)
		mountPath, _ := rawAuth["path"].(string)
		if mountPath == "" {
			mountPath = "approle"
		}
		if roleID == "" || secretID == "" {
			return errors.New("vault backend: auth.type=approle requires role_id and secret_id")
		}
		resp, err := client.Logical().Write("auth/"+mountPath+"/login", map[string]interface{}{
			"role_id":   roleID,
			"secret_id": secretID,
		})
		if err != nil {
			return fmt.Errorf("vault approle login: %w", err)
		}
		if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
			return errors.New("vault approle login: empty auth response")
		}
		client.SetToken(resp.Auth.ClientToken)
		return nil
	case "userpass":
		username, _ := rawAuth["username"].(string)
		password, _ := rawAuth["password"].(string)
		mountPath, _ := rawAuth["path"].(string)
		if mountPath == "" {
			mountPath = "userpass"
		}
		if username == "" || password == "" {
			return errors.New("vault backend: auth.type=userpass requires username and password")
		}
		resp, err := client.Logical().Write("auth/"+mountPath+"/login/"+username, map[string]interface{}{
			"password": password,
		})
		if err != nil {
			return fmt.Errorf("vault userpass login: %w", err)
		}
		if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
			return errors.New("vault userpass login: empty auth response")
		}
		client.SetToken(resp.Auth.ClientToken)
		return nil
	default:
		return fmt.Errorf("vault backend: unsupported auth.type %q (supported: token, approle, userpass)", authType)
	}
}

// secretPath joins the configured prefix with the secret name. Names are
// validated up-front to keep callers from accidentally walking out of the
// prefix into another team's branch.
func (b *Backend) secretPath(name string) string {
	if b.prefix != "" {
		return path.Join(b.prefix, name)
	}
	return name
}

// listPath gives the path used with Logical().List for KV v2 metadata.
func (b *Backend) listPath() string {
	if b.prefix != "" {
		return path.Join(b.mount, "metadata", b.prefix)
	}
	return path.Join(b.mount, "metadata")
}

func validateName(name string) error {
	if name == "" {
		return errors.New("secret name is empty")
	}
	if strings.ContainsAny(name, `\`) || strings.Contains(name, "..") {
		return fmt.Errorf("secret name %q contains forbidden characters", name)
	}
	return nil
}

func (b *Backend) ListSecrets(ctx context.Context) ([]backend.Secret, error) {
	resp, err := b.client.Logical().ListWithContext(ctx, b.listPath())
	if err != nil {
		return nil, fmt.Errorf("vault list: %w", err)
	}
	if resp == nil || resp.Data == nil {
		return []backend.Secret{}, nil
	}
	keys, _ := resp.Data["keys"].([]interface{})
	out := make([]backend.Secret, 0, len(keys))
	for _, k := range keys {
		name, ok := k.(string)
		if !ok || strings.HasSuffix(name, "/") {
			// Sub-folders are skipped: jtsekret keeps a flat namespace.
			continue
		}
		s, err := b.GetSecret(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("vault list: stat %q: %w", name, err)
		}
		out = append(out, *s)
	}
	return out, nil
}

func (b *Backend) GetSecret(ctx context.Context, nameOrID string) (*backend.Secret, error) {
	kv, err := b.client.KVv2(b.mount).GetMetadata(ctx, b.secretPath(nameOrID))
	if err != nil {
		return nil, fmt.Errorf("vault get metadata %q: %w", nameOrID, err)
	}
	if kv == nil {
		return nil, fmt.Errorf("secret not found: %s", nameOrID)
	}
	keys, err := b.entryKeys(ctx, nameOrID)
	if err != nil {
		return nil, err
	}
	createdAt := ""
	if !kv.CreatedTime.IsZero() {
		createdAt = kv.CreatedTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	updatedAt := ""
	if !kv.UpdatedTime.IsZero() {
		updatedAt = kv.UpdatedTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	return &backend.Secret{
		ID:          nameOrID,
		Name:        nameOrID,
		Description: "",
		Labels:      map[string]string{},
		EntryKeys:   keys,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func (b *Backend) GetPayload(ctx context.Context, nameOrID, versionID string) (*backend.Payload, error) {
	kv2 := b.client.KVv2(b.mount)
	var (
		secret *vaultapi.KVSecret
		err    error
	)
	if versionID == "" {
		secret, err = kv2.Get(ctx, b.secretPath(nameOrID))
	} else {
		v, parseErr := strconv.Atoi(versionID)
		if parseErr != nil {
			return nil, fmt.Errorf("vault get %q: invalid version %q: %w", nameOrID, versionID, parseErr)
		}
		secret, err = kv2.GetVersion(ctx, b.secretPath(nameOrID), v)
	}
	if err != nil {
		return nil, fmt.Errorf("vault get %q: %w", nameOrID, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("secret not found: %s", nameOrID)
	}
	entries := make([]backend.Entry, 0, len(secret.Data))
	for k, v := range secret.Data {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("vault get %q: unexpected type %T for key %q (jtsekret stores entries as strings)", nameOrID, v, k)
		}
		entries = append(entries, backend.Entry{Key: k, Value: []byte(s)})
	}
	version := ""
	if secret.VersionMetadata != nil {
		version = strconv.Itoa(secret.VersionMetadata.Version)
	}
	return &backend.Payload{
		SecretID:  nameOrID,
		VersionID: version,
		Entries:   entries,
	}, nil
}

func (b *Backend) CreateSecret(ctx context.Context, name, description string, entries []backend.Entry) (*backend.Secret, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	// Reject duplicates explicitly: KV v2 Put would happily create a v2
	// on top of an existing secret, which is wrong semantics for create.
	if existing, err := b.client.KVv2(b.mount).GetMetadata(ctx, b.secretPath(name)); err == nil && existing != nil {
		return nil, fmt.Errorf("secret %q already exists", name)
	}
	data, err := entriesToData(entries)
	if err != nil {
		return nil, err
	}
	if _, err := b.client.KVv2(b.mount).Put(ctx, b.secretPath(name), data); err != nil {
		return nil, fmt.Errorf("vault put %q: %w", name, err)
	}
	if description != "" {
		_ = b.client.KVv2(b.mount).PutMetadata(ctx, b.secretPath(name), vaultapi.KVMetadataPutInput{
			CustomMetadata: map[string]interface{}{"description": description},
		})
	}
	return b.GetSecret(ctx, name)
}

func (b *Backend) AddVersion(ctx context.Context, nameOrID string, entries []backend.Entry) error {
	if _, err := b.client.KVv2(b.mount).GetMetadata(ctx, b.secretPath(nameOrID)); err != nil {
		return fmt.Errorf("secret not found: %s", nameOrID)
	}
	data, err := entriesToData(entries)
	if err != nil {
		return err
	}
	if _, err := b.client.KVv2(b.mount).Put(ctx, b.secretPath(nameOrID), data); err != nil {
		return fmt.Errorf("vault put %q: %w", nameOrID, err)
	}
	return nil
}

func (b *Backend) DeleteSecret(ctx context.Context, nameOrID string) error {
	if err := b.client.KVv2(b.mount).DeleteMetadata(ctx, b.secretPath(nameOrID)); err != nil {
		return fmt.Errorf("vault delete %q: %w", nameOrID, err)
	}
	return nil
}

// entryKeys is a separate round-trip from GetSecret because GetMetadata
// alone doesn't return the data keys — only versions/timestamps. List
// callers pay this once per secret; that's fine for personal scale.
func (b *Backend) entryKeys(ctx context.Context, name string) ([]string, error) {
	kv, err := b.client.KVv2(b.mount).Get(ctx, b.secretPath(name))
	if err != nil {
		return nil, fmt.Errorf("vault get %q: %w", name, err)
	}
	if kv == nil || kv.Data == nil {
		return []string{}, nil
	}
	out := make([]string, 0, len(kv.Data))
	for k := range kv.Data {
		out = append(out, k)
	}
	return out, nil
}

func entriesToData(entries []backend.Entry) (map[string]interface{}, error) {
	data := make(map[string]interface{}, len(entries))
	for _, e := range entries {
		if !utf8.Valid(e.Value) {
			return nil, fmt.Errorf("entry %q has non-UTF-8 value; the vault backend stores values as JSON strings — "+
				"use the github or file backend for binary data", e.Key)
		}
		data[e.Key] = string(e.Value)
	}
	return data, nil
}
