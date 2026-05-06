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

// Package githubrepo implements a backend that stores secrets as
// AES-256-GCM encrypted files in a git repository (typically a private
// GitHub repo).
//
// On-disk layout inside the repo:
//
//	secrets/
//	  <name>.enc        # [16B salt][12B nonce][AES-GCM(values)]
//	  <name>.meta.json  # plaintext metadata: id, name, description, entry_keys, timestamps, version_id
//
// Values (entry payloads) are encrypted with a key derived via Argon2id
// from the master password and a per-secret salt. Names and entry keys
// are stored in plaintext to make List/Search work without the master
// password (acceptable trade-off for personal repos).
package githubrepo

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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/jtprogru/jtsekret/internal/backend"
	"github.com/jtprogru/jtsekret/internal/crypto"
)

const (
	secretsDir = "secrets"
	metaSuffix = ".meta.json"
	encSuffix  = ".enc"

	defaultBranch = "main"
	commitName    = "jtsekret"
	commitEmail   = "jtsekret@localhost"
)

type Backend struct {
	mu sync.Mutex

	repoURL    string
	branch     string
	localPath  string
	autoPull   bool
	autoPush   bool
	masterPass string
	auth       transport.AuthMethod

	repo *git.Repository
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
	backend.Register("github", New)
}

func New(cfg map[string]interface{}) (backend.Backend, error) {
	repoURL, _ := cfg["repo"].(string)
	if repoURL == "" {
		return nil, errors.New("github backend: repo is required")
	}

	branch, _ := cfg["branch"].(string)
	if branch == "" {
		branch = defaultBranch
	}

	localPath, _ := cfg["local_path"].(string)
	if localPath == "" {
		return nil, errors.New("github backend: local_path is required")
	}

	autoPull := true
	if v, ok := cfg["auto_pull"].(bool); ok {
		autoPull = v
	}
	autoPush := true
	if v, ok := cfg["auto_push"].(bool); ok {
		autoPush = v
	}

	masterPass, _ := cfg["master_password"].(string)
	if masterPass == "" {
		return nil, errors.New("github backend: master password is not set " +
			"(JTSEKRET_GITHUB_MASTER_PASSWORD or JTSEKRET_CACHE_MASTER_PASSWORD)")
	}

	auth, err := buildAuth(cfg, repoURL)
	if err != nil {
		return nil, err
	}

	b := &Backend{
		repoURL:    expandRepoURL(repoURL),
		branch:     branch,
		localPath:  localPath,
		autoPull:   autoPull,
		autoPush:   autoPush,
		masterPass: masterPass,
		auth:       auth,
	}

	if err := b.openOrClone(); err != nil {
		return nil, err
	}

	return b, nil
}

// expandRepoURL converts shorthand "owner/name" to a full HTTPS GitHub URL.
// Full URLs (https://, git@, file://) and absolute paths pass through.
func expandRepoURL(s string) string {
	if strings.Contains(s, "://") || strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "/") {
		return s
	}
	if strings.Count(s, "/") == 1 && !strings.ContainsAny(s, " \t") {
		return "https://github.com/" + s + ".git"
	}
	return s
}

func buildAuth(cfg map[string]interface{}, repoURL string) (transport.AuthMethod, error) {
	rawAuth, ok := cfg["auth"].(map[string]interface{})
	if !ok || rawAuth == nil {
		// Local file:// repos and unauthenticated cases are valid.
		return nil, nil //nolint:nilnil // intentional: nil means "no auth", which go-git treats as anonymous
	}
	authType, _ := rawAuth["type"].(string)
	switch authType {
	case "", "none":
		return nil, nil //nolint:nilnil // explicitly disabled auth
	case "token":
		token, _ := rawAuth["token"].(string)
		if token == "" {
			return nil, errors.New("github backend: auth.type=token requires a token (set JTSEKRET_GITHUB_TOKEN)")
		}
		return &githttp.BasicAuth{Username: "x-access-token", Password: token}, nil
	case "ssh":
		keyPath, _ := rawAuth["ssh_key_path"].(string)
		keyPass, _ := rawAuth["ssh_key_password"].(string)
		user := "git"
		if strings.HasPrefix(repoURL, "git@") {
			if idx := strings.Index(repoURL, "@"); idx > 0 {
				user = repoURL[:idx]
			}
		}
		if keyPath != "" {
			a, err := gitssh.NewPublicKeysFromFile(user, keyPath, keyPass)
			if err != nil {
				return nil, fmt.Errorf("load ssh key: %w", err)
			}
			return a, nil
		}
		a, err := gitssh.NewSSHAgentAuth(user)
		if err != nil {
			return nil, fmt.Errorf("ssh-agent: %w", err)
		}
		return a, nil
	default:
		return nil, fmt.Errorf("unknown github auth type: %q", authType)
	}
}

func (b *Backend) openOrClone() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := os.Stat(filepath.Join(b.localPath, ".git")); err == nil {
		repo, err := git.PlainOpen(b.localPath)
		if err != nil {
			return fmt.Errorf("open repo: %w", err)
		}
		b.repo = repo
		if b.autoPull {
			if err := b.pullLocked(); err != nil {
				return err
			}
		}
		return nil
	}

	if err := os.MkdirAll(b.localPath, 0o700); err != nil {
		return fmt.Errorf("mkdir local_path: %w", err)
	}

	repo, err := git.PlainClone(b.localPath, false, &git.CloneOptions{
		URL:           b.repoURL,
		Auth:          b.auth,
		ReferenceName: plumbing.NewBranchReferenceName(b.branch),
		SingleBranch:  true,
	})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		repo, err = b.initEmpty()
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}
	b.repo = repo
	return nil
}

func (b *Backend) initEmpty() (*git.Repository, error) {
	repo, err := git.PlainInit(b.localPath, false)
	if err != nil {
		return nil, fmt.Errorf("init repo: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{b.repoURL},
	}); err != nil {
		return nil, fmt.Errorf("add remote: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(b.branch),
		Create: true,
	}); err != nil {
		return nil, fmt.Errorf("checkout branch: %w", err)
	}
	return repo, nil
}

func (b *Backend) pullLocked() error {
	wt, err := b.repo.Worktree()
	if err != nil {
		return err
	}
	err = wt.Pull(&git.PullOptions{
		RemoteName:    "origin",
		Auth:          b.auth,
		ReferenceName: plumbing.NewBranchReferenceName(b.branch),
		SingleBranch:  true,
	})
	switch {
	case err == nil, errors.Is(err, git.NoErrAlreadyUpToDate),
		errors.Is(err, transport.ErrEmptyRemoteRepository):
		return nil
	default:
		return fmt.Errorf("pull: %w", err)
	}
}

func (b *Backend) commitAndPush(message string, paths ...string) error {
	wt, err := b.repo.Worktree()
	if err != nil {
		return err
	}
	for _, p := range paths {
		if _, err := wt.Add(p); err != nil {
			return fmt.Errorf("git add %s: %w", p, err)
		}
	}
	_, err = wt.Commit(message, &git.CommitOptions{
		AllowEmptyCommits: false,
		Author: &object.Signature{
			Name:  commitName,
			Email: commitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if !b.autoPush {
		return nil
	}
	err = b.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       b.auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", b.branch, b.branch)),
		},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

// Sync performs an explicit pull then push, regardless of auto_pull/auto_push.
// Useful when the user keeps both flags off and wants manual control.
func (b *Backend) Sync(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.pullLocked(); err != nil {
		return err
	}
	err := b.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       b.auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", b.branch, b.branch)),
		},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

// --- Backend interface ---

func (b *Backend) ListSecrets(ctx context.Context) ([]backend.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := filepath.Join(b.localPath, secretsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []backend.Secret{}, nil
		}
		return nil, fmt.Errorf("read secrets dir: %w", err)
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

	encPath := filepath.Join(b.localPath, secretsDir, nameOrID+encSuffix)
	blob, err := os.ReadFile(encPath)
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
	var payload entriesPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	entries := make([]backend.Entry, len(payload.Entries))
	for i, e := range payload.Entries {
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
	metaPath, encPath, err := b.writeSecret(meta, entries)
	if err != nil {
		return nil, err
	}
	if err := b.commitAndPush("jtsekret: create "+name, metaPath, encPath); err != nil {
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

	metaPath, encPath, err := b.writeSecret(meta, entries)
	if err != nil {
		return err
	}
	return b.commitAndPush(fmt.Sprintf("jtsekret: update %s -> v%s", nameOrID, meta.VersionID), metaPath, encPath)
}

func (b *Backend) DeleteSecret(ctx context.Context, nameOrID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := b.readMeta(nameOrID); err != nil {
		return err
	}
	relMeta := filepath.Join(secretsDir, nameOrID+metaSuffix)
	relEnc := filepath.Join(secretsDir, nameOrID+encSuffix)
	if err := os.Remove(filepath.Join(b.localPath, relMeta)); err != nil {
		return fmt.Errorf("remove meta: %w", err)
	}
	if err := os.Remove(filepath.Join(b.localPath, relEnc)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove enc: %w", err)
	}
	return b.commitAndPush("jtsekret: delete "+nameOrID, relMeta, relEnc)
}

// --- helpers ---

func (b *Backend) readMeta(name string) (*fileMeta, error) {
	path := filepath.Join(b.localPath, secretsDir, name+metaSuffix)
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

func (b *Backend) writeSecret(meta *fileMeta, entries []backend.Entry) (string, string, error) {
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return "", "", fmt.Errorf("salt: %w", err)
	}
	meta.Salt = base64.StdEncoding.EncodeToString(salt)

	key, err := crypto.DeriveKey(b.masterPass, salt)
	if err != nil {
		return "", "", fmt.Errorf("derive key: %w", err)
	}
	items := make([]entryItem, len(entries))
	for i, e := range entries {
		items[i] = entryItem{Key: e.Key, Value: e.Value}
	}
	plaintext, err := json.Marshal(entriesPayload{Entries: items})
	if err != nil {
		return "", "", fmt.Errorf("marshal entries: %w", err)
	}
	ciphertext, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		return "", "", fmt.Errorf("encrypt: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(b.localPath, secretsDir), 0o700); err != nil {
		return "", "", fmt.Errorf("mkdir: %w", err)
	}
	relMeta := filepath.Join(secretsDir, meta.Name+metaSuffix)
	relEnc := filepath.Join(secretsDir, meta.Name+encSuffix)

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := writeAtomic(filepath.Join(b.localPath, relMeta), metaJSON, 0o600); err != nil {
		return "", "", err
	}
	if err := writeAtomic(filepath.Join(b.localPath, relEnc), ciphertext, 0o600); err != nil {
		return "", "", err
	}
	return relMeta, relEnc, nil
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
