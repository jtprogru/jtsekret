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
*/
package githubrepo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/jtprogru/jtsekret/internal/backend"
)

const testPassword = "test-master-password"

// makeBareRemote creates an empty bare repo, plants an initial commit on
// `branch`, and returns the file:// URL of the bare repo.
func makeBareRemote(t *testing.T, branch string) string {
	t.Helper()
	tmp := t.TempDir()
	bareDir := filepath.Join(tmp, "remote.git")
	if _, err := git.PlainInit(bareDir, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	seedDir := filepath.Join(tmp, "seed")
	r, err := git.PlainInit(seedDir, false)
	if err != nil {
		t.Fatalf("init seed: %v", err)
	}
	// Force HEAD to the desired branch before the first commit.
	if err := r.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName(branch),
	)); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(seedDir, "README.md"), []byte("# secrets\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@local", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"file://" + bareDir},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec("refs/heads/" + branch + ":refs/heads/" + branch),
		},
	}); err != nil {
		t.Fatalf("push to bare: %v", err)
	}
	return "file://" + bareDir
}

func newBackend(t *testing.T, remoteURL, branch string) *Backend {
	t.Helper()
	tmp := t.TempDir()
	cfg := map[string]interface{}{
		"repo":            remoteURL,
		"branch":          branch,
		"local_path":      filepath.Join(tmp, "work"),
		"auto_pull":       false,
		"auto_push":       true,
		"master_password": testPassword,
		"auth":            map[string]interface{}{"type": "none"},
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	return b.(*Backend)
}

func TestGithubBackend_CreateGetListUpdateDelete(t *testing.T) {
	remote := makeBareRemote(t, "main")
	b := newBackend(t, remote, "main")
	ctx := context.Background()

	entries := []backend.Entry{
		{Key: "user", Value: []byte("alice")},
		{Key: "pass", Value: []byte("s3cret")},
	}
	s, err := b.CreateSecret(ctx, "api-token", "demo", entries)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.Name != "api-token" || len(s.EntryKeys) != 2 {
		t.Fatalf("unexpected secret: %+v", s)
	}

	list, err := b.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "api-token" {
		t.Fatalf("unexpected list: %+v", list)
	}

	p, err := b.GetPayload(ctx, "api-token", "")
	if err != nil {
		t.Fatalf("get payload: %v", err)
	}
	if p.VersionID != "1" || len(p.Entries) != 2 {
		t.Fatalf("payload: %+v", p)
	}
	got := map[string]string{}
	for _, e := range p.Entries {
		got[e.Key] = string(e.Value)
	}
	if got["user"] != "alice" || got["pass"] != "s3cret" {
		t.Fatalf("decrypt mismatch: %+v", got)
	}

	if err := b.AddVersion(ctx, "api-token", []backend.Entry{
		{Key: "pass", Value: []byte("rotated")},
	}); err != nil {
		t.Fatalf("add version: %v", err)
	}
	p, err = b.GetPayload(ctx, "api-token", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.VersionID != "2" || len(p.Entries) != 1 || string(p.Entries[0].Value) != "rotated" {
		t.Fatalf("after rotate: %+v", p)
	}

	if err := b.DeleteSecret(ctx, "api-token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = b.ListSecrets(ctx)
	if len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}

func TestGithubBackend_WrongPasswordFailsToDecrypt(t *testing.T) {
	remote := makeBareRemote(t, "main")
	b := newBackend(t, remote, "main")
	ctx := context.Background()

	if _, err := b.CreateSecret(ctx, "x", "", []backend.Entry{
		{Key: "k", Value: []byte("v")},
	}); err != nil {
		t.Fatal(err)
	}

	// Open the same remote from a fresh local with a different password.
	tmp := t.TempDir()
	cfg := map[string]interface{}{
		"repo":            remote,
		"branch":          "main",
		"local_path":      filepath.Join(tmp, "work2"),
		"auto_pull":       false,
		"auto_push":       false,
		"master_password": "wrong-password",
		"auth":            map[string]interface{}{"type": "none"},
	}
	b2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b2.GetPayload(ctx, "x", ""); err == nil {
		t.Fatal("expected decrypt failure with wrong password")
	}
}

func TestGithubBackend_RoundtripAcrossClones(t *testing.T) {
	remote := makeBareRemote(t, "main")
	b1 := newBackend(t, remote, "main")
	ctx := context.Background()

	if _, err := b1.CreateSecret(ctx, "shared", "", []backend.Entry{
		{Key: "token", Value: []byte("abc123")},
	}); err != nil {
		t.Fatal(err)
	}

	// Second clone with auto_pull picks up the new secret.
	tmp := t.TempDir()
	cfg := map[string]interface{}{
		"repo":            remote,
		"branch":          "main",
		"local_path":      filepath.Join(tmp, "work2"),
		"auto_pull":       true,
		"auto_push":       false,
		"master_password": testPassword,
		"auth":            map[string]interface{}{"type": "none"},
	}
	b2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, err := b2.GetPayload(ctx, "shared", "")
	if err != nil {
		t.Fatalf("second clone get: %v", err)
	}
	if string(p.Entries[0].Value) != "abc123" {
		t.Fatalf("roundtrip mismatch: %s", p.Entries[0].Value)
	}
}

func TestExpandRepoURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"jtprogru/repo", "https://github.com/jtprogru/repo.git"},
		{"https://github.com/x/y.git", "https://github.com/x/y.git"},
		{"git@github.com:x/y.git", "git@github.com:x/y.git"},
		{"file:///tmp/foo", "file:///tmp/foo"},
		{"/abs/path", "/abs/path"},
	}
	for _, c := range cases {
		if got := expandRepoURL(c.in); got != c.want {
			t.Errorf("expandRepoURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateName(t *testing.T) {
	bad := []string{"", "a/b", `a\b`, "..", "x/..", "../x"}
	for _, n := range bad {
		if err := validateName(n); err == nil {
			t.Errorf("validateName(%q) = nil, want error", n)
		}
	}
	for _, n := range []string{"a", "secret-1", "my.token", "Long_Name_42"} {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", n, err)
		}
	}
}
