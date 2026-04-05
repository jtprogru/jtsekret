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
	"testing"

	"github.com/jtprogru/jtsekret/internal/backend"
)

func TestMockBackend_CreateSecret(t *testing.T) {
	b, err := NewBackend(nil)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	ctx := context.Background()
	entries := []backend.Entry{
		{Key: "token", Value: []byte("secret-token")},
		{Key: "api_key", Value: []byte("api-key-123")},
	}

	secret, err := b.CreateSecret(ctx, "my-secret", "Test secret", entries)
	if err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	if secret.Name != "my-secret" {
		t.Errorf("CreateSecret() name = %v, want %v", secret.Name, "my-secret")
	}
	if len(secret.EntryKeys) != 2 {
		t.Errorf("CreateSecret() entry keys = %v, want %v", len(secret.EntryKeys), 2)
	}
}

func TestMockBackend_GetPayload(t *testing.T) {
	b, err := NewBackend(nil)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	ctx := context.Background()
	entries := []backend.Entry{
		{Key: "password", Value: []byte("hunter2")},
	}

	_, err = b.CreateSecret(ctx, "db-creds", "Database credentials", entries)
	if err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	payload, err := b.GetPayload(ctx, "db-creds", "")
	if err != nil {
		t.Fatalf("GetPayload() error = %v", err)
	}

	if len(payload.Entries) != 1 {
		t.Errorf("GetPayload() entries = %v, want %v", len(payload.Entries), 1)
	}
	if payload.Entries[0].Key != "password" {
		t.Errorf("GetPayload() key = %v, want %v", payload.Entries[0].Key, "password")
	}
}

func TestMockBackend_AddVersion(t *testing.T) {
	b, err := NewBackend(nil)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	ctx := context.Background()
	entries := []backend.Entry{
		{Key: "token", Value: []byte("old-token")},
	}

	secret, err := b.CreateSecret(ctx, "api-token", "API token", entries)
	if err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	newEntries := []backend.Entry{
		{Key: "token", Value: []byte("new-token")},
	}
	err = b.AddVersion(ctx, secret.ID, newEntries)
	if err != nil {
		t.Fatalf("AddVersion() error = %v", err)
	}

	payload, err := b.GetPayload(ctx, secret.ID, "")
	if err != nil {
		t.Fatalf("GetPayload() error = %v", err)
	}

	if string(payload.Entries[0].Value) != "new-token" {
		t.Errorf("AddVersion() value = %v, want %v", string(payload.Entries[0].Value), "new-token")
	}
}

func TestMockBackend_DeleteSecret(t *testing.T) {
	b, err := NewBackend(nil)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	ctx := context.Background()
	entries := []backend.Entry{
		{Key: "key", Value: []byte("value")},
	}

	secret, err := b.CreateSecret(ctx, "to-delete", "Will be deleted", entries)
	if err != nil {
		t.Fatalf("CreateSecret() error = %v", err)
	}

	err = b.DeleteSecret(ctx, secret.ID)
	if err != nil {
		t.Fatalf("DeleteSecret() error = %v", err)
	}

	_, err = b.GetSecret(ctx, secret.ID)
	if err == nil {
		t.Error("DeleteSecret() should return error for deleted secret")
	}
}

func TestMockBackend_ListSecrets(t *testing.T) {
	b, err := NewBackend(nil)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	ctx := context.Background()

	_, _ = b.CreateSecret(ctx, "secret-1", "First", []backend.Entry{{Key: "k1", Value: []byte("v1")}})
	_, _ = b.CreateSecret(ctx, "secret-2", "Second", []backend.Entry{{Key: "k2", Value: []byte("v2")}})

	secrets, err := b.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}

	if len(secrets) != 2 {
		t.Errorf("ListSecrets() count = %v, want %v", len(secrets), 2)
	}
}
