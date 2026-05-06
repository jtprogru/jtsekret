/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
)

// fakeKV is the minimum slice of the Vault KV v2 HTTP API jtsekret touches.
// Per-secret state holds versions in a slice and the current_version
// pointer, mirroring real Vault enough for our integration with the SDK.
type fakeKV struct {
	mu    sync.Mutex
	store map[string]*kvSecret // path -> secret
}

type kvSecret struct {
	versions  []map[string]interface{}
	createdAt time.Time
	updatedAt time.Time
	deleted   bool
}

func newFakeVault(t *testing.T) *httptest.Server {
	t.Helper()
	fv := &fakeKV{store: map[string]*kvSecret{}}
	s := httptest.NewServer(http.HandlerFunc(fv.handle))
	t.Cleanup(s.Close)
	return s
}

func (fv *fakeKV) handle(w http.ResponseWriter, r *http.Request) {
	fv.mu.Lock()
	defer fv.mu.Unlock()

	const dataPrefix = "/v1/secret/data/"
	const metaPrefix = "/v1/secret/metadata/"

	switch {
	case r.Method == http.MethodGet && r.URL.Query().Get("list") == "true" && strings.HasPrefix(r.URL.Path, metaPrefix):
		base := strings.TrimPrefix(r.URL.Path, metaPrefix)
		base = strings.TrimSuffix(base, "/")
		var keys []string
		for k, sec := range fv.store {
			if sec.deleted {
				continue
			}
			if base == "" || strings.HasPrefix(k, base+"/") {
				name := strings.TrimPrefix(k, base+"/")
				if base == "" {
					name = k
				}
				keys = append(keys, name)
			}
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"keys": keys}})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, metaPrefix):
		key := strings.TrimPrefix(r.URL.Path, metaPrefix)
		sec, ok := fv.store[key]
		if !ok || sec.deleted {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{
			"created_time": sec.createdAt.UTC().Format(time.RFC3339Nano),
			"updated_time": sec.updatedAt.UTC().Format(time.RFC3339Nano),
			"current_version": len(sec.versions),
			"versions":        map[string]interface{}{}, // detailed map not needed by jtsekret
		}})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, dataPrefix):
		key := strings.TrimPrefix(r.URL.Path, dataPrefix)
		sec, ok := fv.store[key]
		if !ok || sec.deleted || len(sec.versions) == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		idx := len(sec.versions) - 1
		if v := r.URL.Query().Get("version"); v != "" {
			n, _ := strconv.Atoi(v)
			if n < 1 || n > len(sec.versions) {
				http.Error(w, "version out of range", http.StatusNotFound)
				return
			}
			idx = n - 1
		}
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{
			"data":     sec.versions[idx],
			"metadata": map[string]interface{}{"version": idx + 1},
		}})
	case (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.HasPrefix(r.URL.Path, dataPrefix):
		key := strings.TrimPrefix(r.URL.Path, dataPrefix)
		var body struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sec, ok := fv.store[key]
		now := time.Now()
		if !ok {
			sec = &kvSecret{createdAt: now}
			fv.store[key] = sec
		}
		sec.deleted = false
		sec.versions = append(sec.versions, body.Data)
		sec.updatedAt = now
		writeJSON(w, map[string]interface{}{"data": map[string]interface{}{"version": len(sec.versions)}})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, metaPrefix):
		key := strings.TrimPrefix(r.URL.Path, metaPrefix)
		if sec, ok := fv.store[key]; ok {
			sec.deleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	case (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.HasPrefix(r.URL.Path, metaPrefix):
		// PutMetadata — accept and ignore (description tracking not needed by tests).
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "fake-vault: not implemented "+r.Method+" "+r.URL.Path, http.StatusNotImplemented)
	}
}

func writeJSON(w http.ResponseWriter, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func newBackend(t *testing.T, srv *httptest.Server) *Backend {
	t.Helper()
	cfg := map[string]interface{}{
		"address": srv.URL,
		"mount":   "secret",
		"prefix":  "personal",
		"auth":    map[string]interface{}{"type": "token", "token": "fake-token"},
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return b.(*Backend)
}

func TestVault_CRUD(t *testing.T) {
	srv := newFakeVault(t)
	b := newBackend(t, srv)
	ctx := context.Background()

	entries := []backend.Entry{
		{Key: "user", Value: []byte("alice")},
		{Key: "pass", Value: []byte("s3cret")},
	}
	if _, err := b.CreateSecret(ctx, "demo", "demo desc", entries); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := b.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "demo" {
		t.Fatalf("list: %+v", list)
	}
	p, err := b.GetPayload(ctx, "demo", "")
	if err != nil {
		t.Fatalf("get payload: %v", err)
	}
	got := map[string]string{}
	for _, e := range p.Entries {
		got[e.Key] = string(e.Value)
	}
	if got["user"] != "alice" || got["pass"] != "s3cret" {
		t.Fatalf("payload mismatch: %+v", got)
	}
	if p.VersionID != "1" {
		t.Fatalf("version: %s", p.VersionID)
	}

	if err := b.AddVersion(ctx, "demo", []backend.Entry{{Key: "tok", Value: []byte("rotated")}}); err != nil {
		t.Fatalf("add version: %v", err)
	}
	p, _ = b.GetPayload(ctx, "demo", "")
	if p.VersionID != "2" || string(p.Entries[0].Value) != "rotated" {
		t.Fatalf("after rotate: %+v", p)
	}
	pV1, err := b.GetPayload(ctx, "demo", "1")
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	v1 := map[string]string{}
	for _, e := range pV1.Entries {
		v1[e.Key] = string(e.Value)
	}
	if v1["user"] != "alice" {
		t.Fatalf("v1 retained: %+v", v1)
	}

	if err := b.DeleteSecret(ctx, "demo"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = b.ListSecrets(ctx)
	if len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}

func TestVault_DuplicateCreateRejected(t *testing.T) {
	srv := newFakeVault(t)
	b := newBackend(t, srv)
	ctx := context.Background()
	if _, err := b.CreateSecret(ctx, "x", "", []backend.Entry{{Key: "k", Value: []byte("v")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateSecret(ctx, "x", "", []backend.Entry{{Key: "k", Value: []byte("v2")}}); err == nil {
		t.Fatal("expected duplicate-create error")
	}
}

func TestVault_BinaryRejected(t *testing.T) {
	srv := newFakeVault(t)
	b := newBackend(t, srv)
	ctx := context.Background()
	bad := []byte{0xff, 0x00, 0xfe, 0xc3, 0x28}
	if _, err := b.CreateSecret(ctx, "bin", "", []backend.Entry{{Key: "k", Value: bad}}); err == nil {
		t.Fatal("expected non-UTF-8 rejection")
	}
}

func TestVault_AuthTokenRequired(t *testing.T) {
	cfg := map[string]interface{}{
		"address": "http://127.0.0.1:1",
		"auth":    map[string]interface{}{"type": "token"},
	}
	if _, err := New(cfg); err == nil {
		t.Fatal("expected auth.token requirement")
	}
}

func TestValidateName(t *testing.T) {
	bad := []string{"", `a\b`, "..", "x/..", "../x"}
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
