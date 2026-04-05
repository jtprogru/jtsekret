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
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
)

type MockBackend struct {
	mu       sync.RWMutex
	secrets  map[string]*backend.Secret
	payloads map[string]*backend.Payload
}

func NewBackend(cfg map[string]interface{}) (backend.Backend, error) {
	return &MockBackend{
		secrets:  make(map[string]*backend.Secret),
		payloads: make(map[string]*backend.Payload),
	}, nil
}

func (m *MockBackend) ListSecrets(ctx context.Context) ([]backend.Secret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	secrets := make([]backend.Secret, 0, len(m.secrets))
	for _, s := range m.secrets {
		secrets = append(secrets, *s)
	}

	return secrets, nil
}

func (m *MockBackend) GetSecret(ctx context.Context, nameOrID string) (*backend.Secret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	secret, ok := m.secrets[nameOrID]
	if !ok {
		return nil, fmt.Errorf("secret not found: %s", nameOrID)
	}

	return secret, nil
}

func (m *MockBackend) GetPayload(ctx context.Context, nameOrID string, versionID string) (*backend.Payload, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	payload, ok := m.payloads[nameOrID]
	if !ok {
		return nil, fmt.Errorf("payload not found: %s", nameOrID)
	}

	return payload, nil
}

func (m *MockBackend) CreateSecret(ctx context.Context, name, description string, entries []backend.Entry) (*backend.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("secret-%d", len(m.secrets)+1)
	now := time.Now().Format(time.RFC3339)

	secret := &backend.Secret{
		ID:          id,
		Name:        name,
		Description: description,
		Labels:      map[string]string{},
		EntryKeys:   make([]string, len(entries)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	for i, e := range entries {
		secret.EntryKeys[i] = e.Key
	}

	m.secrets[id] = secret

	if len(entries) > 0 {
		m.payloads[id] = &backend.Payload{
			SecretID:  id,
			VersionID: "1",
			Entries:   entries,
		}
	}

	return secret, nil
}

func (m *MockBackend) AddVersion(ctx context.Context, nameOrID string, entries []backend.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.secrets[nameOrID]; !ok {
		return fmt.Errorf("secret not found: %s", nameOrID)
	}

	now := time.Now().Format(time.RFC3339)
	m.secrets[nameOrID].UpdatedAt = now

	entryKeys := make([]string, len(entries))
	for i, e := range entries {
		entryKeys[i] = e.Key
	}
	m.secrets[nameOrID].EntryKeys = entryKeys

	m.payloads[nameOrID] = &backend.Payload{
		SecretID:  nameOrID,
		VersionID: "2",
		Entries:   entries,
	}

	return nil
}

func (m *MockBackend) DeleteSecret(ctx context.Context, nameOrID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.secrets[nameOrID]; !ok {
		return fmt.Errorf("secret not found: %s", nameOrID)
	}

	delete(m.secrets, nameOrID)
	delete(m.payloads, nameOrID)

	return nil
}

func init() {
	backend.Register("mock", NewBackend)
}
