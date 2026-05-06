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

// Package file implements a fully-offline backend that stores secrets as
// AES-256-GCM encrypted files on the local filesystem. Same on-disk
// layout as the github backend (secrets/<name>.{enc,meta.json}); the
// only difference is that there is no git layer on top — useful for
// air-gapped use, single-device personal storage, and as a destination
// for `jtsekret migrate` exports/imports.
package file

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/crypto"
)

const (
	secretsDir = "secrets"
	metaSuffix = ".meta.json"
	encSuffix  = ".enc"
)

type Backend struct {
	mu         sync.Mutex
	root       string
	masterPass string
}

type fileMeta struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	EntryKeys   []string          `json:"entry_keys"`
	VersionID   string            `json:"version_id"`
	Salt        string            `json:"salt"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type entriesPayload struct {
	Entries []entryItem `json:"entries"`
}

type entryItem struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

func init() {
	backend.Register("file", New)
}

func New(cfg map[string]interface{}) (backend.Backend, error) {
	root, _ := cfg["path"].(string)
	if root == "" {
		return nil, errors.New("file backend: path is required")
	}
	masterPass, _ := cfg["master_password"].(string)
	if masterPass == "" {
		return nil, errors.New(
			"file backend: master password is not set " +
				"(JTSEKRET_FILE_MASTER_PASSWORD or JTSEKRET_CACHE_MASTER_PASSWORD)")
	}
	if err := os.MkdirAll(filepath.Join(root, secretsDir), 0o700); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	return &Backend{root: root, masterPass: masterPass}, nil
}

func (b *Backend) ListSecrets(ctx context.Context) ([]backend.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := filepath.Join(b.root, secretsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []backend.Secret{}, nil
		}
		return nil, fmt.Errorf("read store dir: %w", err)
	}
	out := make([]backend.Secret, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), metaSuffix) {
			continue
		}
		name := strings.TrimSuffix(e.Name(), metaSuffix)
		meta, err := b.readMeta(name)
		if err != nil {
			return nil, fmt.Errorf("read meta %q: %w", name, err)
		}
		out = append(out, metaToSecret(meta))
	}
	return out, nil
}

func (b *Backend) GetSecret(ctx context.Context, nameOrID string) (*backend.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	meta, err := b.readMeta(nameOrID)
	if err != nil {
		return nil, err
	}
	s := metaToSecret(meta)
	return &s, nil
}

func (b *Backend) GetPayload(ctx context.Context, nameOrID, versionID string) (*backend.Payload, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	meta, err := b.readMeta(nameOrID)
	if err != nil {
		return nil, err
	}
	if versionID != "" && versionID != meta.VersionID {
		return nil, fmt.Errorf("version %q not found (only current version is stored)", versionID)
	}
	blob, err := os.ReadFile(filepath.Join(b.root, secretsDir, nameOrID+encSuffix))
	if err != nil {
		return nil, fmt.Errorf("read enc: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(meta.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	key, err := crypto.DeriveKey(b.masterPass, salt)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	plaintext, err := crypto.Decrypt(blob, key)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	var p entriesPayload
	if err := json.Unmarshal(plaintext, &p); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	entries := make([]backend.Entry, len(p.Entries))
	for i, e := range p.Entries {
		entries[i] = backend.Entry{Key: e.Key, Value: e.Value}
	}
	return &backend.Payload{
		SecretID:  meta.ID,
		VersionID: meta.VersionID,
		Entries:   entries,
	}, nil
}

func (b *Backend) CreateSecret(ctx context.Context, name, description string, entries []backend.Entry) (*backend.Secret, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := b.readMeta(name); err == nil {
		return nil, fmt.Errorf("secret %q already exists", name)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	meta := &fileMeta{
		ID:          name,
		Name:        name,
		Description: description,
		Labels:      map[string]string{},
		EntryKeys:   keysOf(entries),
		VersionID:   "1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := b.writeSecret(meta, entries); err != nil {
		return nil, err
	}
	s := metaToSecret(meta)
	return &s, nil
}

func (b *Backend) AddVersion(ctx context.Context, nameOrID string, entries []backend.Entry) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	meta, err := b.readMeta(nameOrID)
	if err != nil {
		return err
	}
	v, _ := strconv.Atoi(meta.VersionID)
	if v < 1 {
		v = 1
	}
	meta.VersionID = strconv.Itoa(v + 1)
	meta.EntryKeys = keysOf(entries)
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return b.writeSecret(meta, entries)
}

// RotateMasterPassword re-encrypts every secret in the store under
// newPassword. Each secret gets a fresh salt so the resulting ciphertext
// shares nothing with the old key. The operation is best-effort
// per-secret: if one fails halfway, every secret rewritten before the
// failure is already under newPassword (their meta.Salt has changed),
// so the rotation is forward-only — there's no rollback to the old
// password without restoring from a backup.
func (b *Backend) RotateMasterPassword(ctx context.Context, newPassword string) error {
	if newPassword == "" {
		return errors.New("new master password is empty")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := filepath.Join(b.root, secretsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			b.masterPass = newPassword
			return nil
		}
		return fmt.Errorf("read store dir: %w", err)
	}
	// Decrypt under the original password (captured here), write under
	// the new one. Writes happen via b.writeSecret which reads
	// b.masterPass — flip it once and never flip back; the loop's
	// decrypt step uses the local oldPass variable, not the field.
	oldPass := b.masterPass
	b.masterPass = newPassword
	rotated := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), metaSuffix) {
			continue
		}
		name := strings.TrimSuffix(e.Name(), metaSuffix)
		meta, err := b.readMeta(name)
		if err != nil {
			return fmt.Errorf("rotate %q: read meta: %w (rotated %d before failure)", name, err, rotated)
		}
		blob, err := os.ReadFile(filepath.Join(b.root, secretsDir, name+encSuffix))
		if err != nil {
			return fmt.Errorf("rotate %q: read enc: %w", name, err)
		}
		oldSalt, err := base64.StdEncoding.DecodeString(meta.Salt)
		if err != nil {
			return fmt.Errorf("rotate %q: decode salt: %w", name, err)
		}
		oldKey, err := crypto.DeriveKey(oldPass, oldSalt)
		if err != nil {
			return fmt.Errorf("rotate %q: derive old key: %w", name, err)
		}
		plaintext, err := crypto.Decrypt(blob, oldKey)
		if err != nil {
			return fmt.Errorf("rotate %q: decrypt with current password: %w", name, err)
		}
		var p entriesPayload
		if err := json.Unmarshal(plaintext, &p); err != nil {
			return fmt.Errorf("rotate %q: unmarshal: %w", name, err)
		}
		decoded := make([]backend.Entry, len(p.Entries))
		for i, e := range p.Entries {
			decoded[i] = backend.Entry{Key: e.Key, Value: e.Value}
		}
		if err := b.writeSecret(meta, decoded); err != nil {
			return fmt.Errorf("rotate %q: rewrite under new password: %w (rotated %d before failure)", name, err, rotated)
		}
		rotated++
	}
	return nil
}

func (b *Backend) DeleteSecret(ctx context.Context, nameOrID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := b.readMeta(nameOrID); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(b.root, secretsDir, nameOrID+metaSuffix)); err != nil {
		return fmt.Errorf("remove meta: %w", err)
	}
	if err := os.Remove(filepath.Join(b.root, secretsDir, nameOrID+encSuffix)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove enc: %w", err)
	}
	return nil
}

// --- helpers ---

func (b *Backend) readMeta(name string) (*fileMeta, error) {
	path := filepath.Join(b.root, secretsDir, name+metaSuffix)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("secret not found: %s", name)
		}
		return nil, fmt.Errorf("read meta: %w", err)
	}
	var meta fileMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal meta: %w", err)
	}
	return &meta, nil
}

func (b *Backend) writeSecret(meta *fileMeta, entries []backend.Entry) error {
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("salt: %w", err)
	}
	meta.Salt = base64.StdEncoding.EncodeToString(salt)

	key, err := crypto.DeriveKey(b.masterPass, salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}
	items := make([]entryItem, len(entries))
	for i, e := range entries {
		items[i] = entryItem{Key: e.Key, Value: e.Value}
	}
	plaintext, err := json.Marshal(entriesPayload{Entries: items})
	if err != nil {
		return fmt.Errorf("marshal entries: %w", err)
	}
	ciphertext, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(b.root, secretsDir), 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(b.root, secretsDir, meta.Name+metaSuffix), metaJSON, 0o600); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(b.root, secretsDir, meta.Name+encSuffix), ciphertext, 0o600)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".jtsekret-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func metaToSecret(m *fileMeta) backend.Secret {
	return backend.Secret{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Labels:      m.Labels,
		EntryKeys:   append([]string(nil), m.EntryKeys...),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func keysOf(entries []backend.Entry) []string {
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = e.Key
	}
	return keys
}

func validateName(name string) error {
	if name == "" {
		return errors.New("secret name is empty")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("secret name %q contains forbidden characters", name)
	}
	return nil
}
