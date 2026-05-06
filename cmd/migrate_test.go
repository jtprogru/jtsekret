/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/backend/mock"
)

func newMock(t *testing.T) backend.Backend {
	t.Helper()
	b, err := mock.NewBackend(nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMigrateBackends_CreateNew(t *testing.T) {
	ctx := context.Background()
	src := newMock(t)
	tgt := newMock(t)

	for i, name := range []string{"alpha", "beta", "gamma"} {
		_, err := src.CreateSecret(ctx, name, "", []backend.Entry{
			{Key: "k", Value: []byte{byte('a' + i)}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	stats, err := MigrateBackends(ctx, src, tgt, MigrateOptions{Out: &buf})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if stats.Created != 3 || stats.Updated != 0 || stats.Skipped != 0 {
		t.Fatalf("stats: %+v", stats)
	}
	tlist, _ := tgt.ListSecrets(ctx)
	if len(tlist) != 3 {
		t.Fatalf("target has %d secrets, want 3", len(tlist))
	}
}

func TestMigrateBackends_CollisionFailsWithoutUpdate(t *testing.T) {
	ctx := context.Background()
	src := newMock(t)
	tgt := newMock(t)
	_, _ = src.CreateSecret(ctx, "collide", "", []backend.Entry{{Key: "k", Value: []byte("new")}})
	_, _ = tgt.CreateSecret(ctx, "collide", "", []backend.Entry{{Key: "k", Value: []byte("old")}})

	var buf bytes.Buffer
	_, err := MigrateBackends(ctx, src, tgt, MigrateOptions{Out: &buf})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected collision error, got %v", err)
	}
}

func TestMigrateBackends_UpdateOverwrites(t *testing.T) {
	ctx := context.Background()
	src := newMock(t)
	tgt := newMock(t)
	_, _ = src.CreateSecret(ctx, "x", "", []backend.Entry{{Key: "k", Value: []byte("new")}})
	_, _ = tgt.CreateSecret(ctx, "x", "", []backend.Entry{{Key: "k", Value: []byte("old")}})

	var buf bytes.Buffer
	stats, err := MigrateBackends(ctx, src, tgt, MigrateOptions{Update: true, Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Updated != 1 || stats.Created != 0 {
		t.Fatalf("stats: %+v", stats)
	}
	p, _ := tgt.GetPayload(ctx, "x", "")
	if string(p.Entries[0].Value) != "new" {
		t.Fatalf("not updated: %s", p.Entries[0].Value)
	}
}

func TestMigrateBackends_DryRunNoWrite(t *testing.T) {
	ctx := context.Background()
	src := newMock(t)
	tgt := newMock(t)
	_, _ = src.CreateSecret(ctx, "a", "", []backend.Entry{{Key: "k", Value: []byte("v")}})

	var buf bytes.Buffer
	if _, err := MigrateBackends(ctx, src, tgt, MigrateOptions{DryRun: true, Out: &buf}); err != nil {
		t.Fatal(err)
	}
	tlist, _ := tgt.ListSecrets(ctx)
	if len(tlist) != 0 {
		t.Fatalf("dry run wrote %d secrets", len(tlist))
	}
	if !strings.Contains(buf.String(), "DRY-RUN  a") {
		t.Fatalf("missing dry-run line: %q", buf.String())
	}
}

func TestMigrateBackends_OnlyFiltersSubset(t *testing.T) {
	ctx := context.Background()
	src := newMock(t)
	tgt := newMock(t)
	for _, n := range []string{"keep", "drop1", "drop2"} {
		_, _ = src.CreateSecret(ctx, n, "", []backend.Entry{{Key: "k", Value: []byte(n)}})
	}

	var buf bytes.Buffer
	stats, err := MigrateBackends(ctx, src, tgt, MigrateOptions{Only: []string{"keep"}, Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 1 || stats.Skipped != 2 {
		t.Fatalf("stats: %+v", stats)
	}
	tlist, _ := tgt.ListSecrets(ctx)
	if len(tlist) != 1 || tlist[0].Name != "keep" {
		t.Fatalf("target: %+v", tlist)
	}
}
