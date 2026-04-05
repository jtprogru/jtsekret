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
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jtprogru/jtsekret/internal/crypto"
	"github.com/jtprogru/jtsekret/internal/domain"
)

type EncryptedFile struct {
	filePath string
	password []byte
	data     *CacheData
}

func NewEncryptedFile(filePath string, password string) (*EncryptedFile, error) {
	absPath, err := expandPath(filePath)
	if err != nil {
		return nil, fmt.Errorf("expand path: %w", err)
	}

	ef := &EncryptedFile{
		filePath: absPath,
		password: []byte(password),
		data:     &CacheData{Version: 1, Entries: make(map[string]CacheEntry)},
	}

	if err := ef.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load cache: %w", err)
		}
	}

	return ef, nil
}

func expandPath(path string) (string, error) {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil
}

func (e *EncryptedFile) load() error {
	ciphertext, err := os.ReadFile(e.filePath)
	if err != nil {
		return err
	}

	if len(ciphertext) < crypto.SaltSize+crypto.NonceSize {
		return fmt.Errorf("invalid cache file")
	}

	salt := ciphertext[:crypto.SaltSize]
	ciphertext = ciphertext[crypto.SaltSize:]

	key, err := crypto.DeriveKey(string(e.password), salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	plaintext, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		return fmt.Errorf("decrypt cache: %w", err)
	}

	var data CacheData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return fmt.Errorf("unmarshal cache: %w", err)
	}

	e.data = &data
	return nil
}

func (e *EncryptedFile) save() error {
	plaintext, err := json.Marshal(e.data)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	key, err := crypto.DeriveKey(string(e.password), salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	ciphertext, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		return fmt.Errorf("encrypt cache: %w", err)
	}

	combined := make([]byte, 0, len(salt)+len(ciphertext))
	combined = append(combined, salt...)
	combined = append(combined, ciphertext...)

	dir := filepath.Dir(e.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "cache-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(combined); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), e.filePath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func (e *EncryptedFile) Get(ctx context.Context, nameOrID string) (*domain.CachedPayload, error) {
	entry, ok := e.data.Entries[nameOrID]
	if !ok {
		return nil, nil
	}

	expiresAt := entry.CachedAt.Add(time.Duration(entry.TTLSeconds) * time.Second)
	if time.Now().After(expiresAt) {
		delete(e.data.Entries, nameOrID)
		_ = e.save()
		return nil, nil
	}

	payload := &domain.CachedPayload{
		Entries:  make(map[string][]byte),
		CachedAt: entry.CachedAt,
		TTL:      time.Duration(entry.TTLSeconds) * time.Second,
	}

	for _, ent := range entry.Payload.Entries {
		payload.Entries[ent.Key] = ent.Value
	}

	return payload, nil
}

func (e *EncryptedFile) Set(ctx context.Context, nameOrID string, payload *domain.CachedPayload) error {
	entries := make([]domain.Entry, 0, len(payload.Entries))
	for k, v := range payload.Entries {
		entries = append(entries, domain.Entry{Key: k, Value: v})
	}

	e.data.Entries[nameOrID] = CacheEntry{
		Payload: &domain.Payload{
			SecretID:  nameOrID,
			VersionID: "1",
			Entries:   entries,
		},
		CachedAt:   payload.CachedAt,
		TTLSeconds: int(payload.TTL.Seconds()),
	}

	return e.save()
}

func (e *EncryptedFile) Delete(ctx context.Context, nameOrID string) error {
	if _, ok := e.data.Entries[nameOrID]; !ok {
		return nil
	}

	delete(e.data.Entries, nameOrID)
	return e.save()
}

func (e *EncryptedFile) Clear(ctx context.Context) error {
	e.data.Entries = make(map[string]CacheEntry)
	return e.save()
}

func (e *EncryptedFile) Stats(ctx context.Context) (map[string]interface{}, error) {
	now := time.Now()
	valid := 0
	expired := 0

	for _, entry := range e.data.Entries {
		expiresAt := entry.CachedAt.Add(time.Duration(entry.TTLSeconds) * time.Second)
		if now.After(expiresAt) {
			expired++
		} else {
			valid++
		}
	}

	return map[string]interface{}{
		"total":   len(e.data.Entries),
		"valid":   valid,
		"expired": expired,
		"file":    e.filePath,
	}, nil
}
