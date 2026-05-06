/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package file

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jtprogru/jtsekret/internal/backend"
)

const testPassword = "test-master-password"

func newBackend(t *testing.T) *Backend {
	t.Helper()
	tmp := t.TempDir()
	cfg := map[string]interface{}{
		"path":            filepath.Join(tmp, "store"),
		"master_password": testPassword,
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return b.(*Backend)
}

func TestFile_CRUD(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)

	entries := []backend.Entry{
		{Key: "user", Value: []byte("alice")},
		{Key: "pass", Value: []byte("s3cret")},
	}
	if _, err := b.CreateSecret(ctx, "demo", "", entries); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := b.ListSecrets(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "demo" {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	p, err := b.GetPayload(ctx, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range p.Entries {
		got[e.Key] = string(e.Value)
	}
	if got["user"] != "alice" || got["pass"] != "s3cret" {
		t.Fatalf("payload mismatch: %+v", got)
	}

	if err := b.AddVersion(ctx, "demo", []backend.Entry{{Key: "tok", Value: []byte("rotated")}}); err != nil {
		t.Fatal(err)
	}
	p, _ = b.GetPayload(ctx, "demo", "")
	if p.VersionID != "2" || len(p.Entries) != 1 || string(p.Entries[0].Value) != "rotated" {
		t.Fatalf("after rotate: %+v", p)
	}
	if err := b.DeleteSecret(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	list, _ = b.ListSecrets(ctx)
	if len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}

func TestFile_WrongPasswordFailsToDecrypt(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	if _, err := b.CreateSecret(ctx, "x", "", []backend.Entry{{Key: "k", Value: []byte("v")}}); err != nil {
		t.Fatal(err)
	}

	cfg := map[string]interface{}{
		"path":            b.root,
		"master_password": "wrong-password",
	}
	b2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b2.GetPayload(ctx, "x", ""); err == nil {
		t.Fatal("expected decrypt failure with wrong password")
	}
}

func TestFile_DuplicateCreate(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	_, _ = b.CreateSecret(ctx, "dup", "", []backend.Entry{{Key: "k", Value: []byte("v")}})
	if _, err := b.CreateSecret(ctx, "dup", "", []backend.Entry{{Key: "k", Value: []byte("v2")}}); err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestFile_BinaryValue(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	raw := []byte{0xff, 0x00, 0x01, 0xfe, 0xc3, 0x28} // invalid UTF-8
	if _, err := b.CreateSecret(ctx, "bin", "", []backend.Entry{{Key: "k", Value: raw}}); err != nil {
		t.Fatal(err)
	}
	p, err := b.GetPayload(ctx, "bin", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Entries[0].Value) != string(raw) {
		t.Fatalf("binary roundtrip mismatch")
	}
}
