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
package backend

import (
	"context"
	"fmt"

	"github.com/jtprogru/jtsekret/internal/domain"
)

type Entry struct {
	Key   string
	Value []byte
}

type Secret struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	EntryKeys   []string          `json:"entry_keys"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type Payload struct {
	SecretID   string            `json:"secret_id"`
	VersionID  string            `json:"version_id"`
	Entries    []Entry           `json:"entries"`
	EntriesMap map[string][]byte `json:"-"`
}

type Backend interface {
	ListSecrets(ctx context.Context) ([]Secret, error)
	GetSecret(ctx context.Context, nameOrID string) (*Secret, error)
	GetPayload(ctx context.Context, nameOrID string, versionID string) (*Payload, error)
	CreateSecret(ctx context.Context, name, description string, entries []Entry) (*Secret, error)
	AddVersion(ctx context.Context, nameOrID string, entries []Entry) error
	DeleteSecret(ctx context.Context, nameOrID string) error
}

// Syncer is implemented by backends that have an explicit synchronization
// step with a remote (e.g. git pull/push). Backends that always operate
// against an authoritative remote (Lockbox, Vault) don't need this.
type Syncer interface {
	Sync(ctx context.Context) error
}

type Factory func(cfg map[string]interface{}) (Backend, error)

var registry = map[string]Factory{}

func Register(name string, f Factory) {
	registry[name] = f
}

func New(backendType string, cfg map[string]interface{}) (Backend, error) {
	f, ok := registry[backendType]
	if !ok {
		return nil, fmt.Errorf("unknown backend: %s", backendType)
	}
	return f(cfg)
}

func Registered() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

func SecretToDomain(s *Secret) *domain.Secret {
	entryKeys := make([]string, len(s.EntryKeys))
	copy(entryKeys, s.EntryKeys)
	return &domain.Secret{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Labels:      s.Labels,
		EntryKeys:   entryKeys,
	}
}

func PayloadToDomain(p *Payload) *domain.Payload {
	entries := make([]domain.Entry, len(p.Entries))
	for i, e := range p.Entries {
		entries[i] = domain.Entry{Key: e.Key, Value: e.Value}
	}
	return &domain.Payload{
		SecretID:  p.SecretID,
		VersionID: p.VersionID,
		Entries:   entries,
	}
}
