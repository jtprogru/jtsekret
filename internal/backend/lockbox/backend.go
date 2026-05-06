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
package lockbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/jtprogru/jtsekret/internal/backend"
	lockboxpb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
	payloadsvc "github.com/yandex-cloud/go-sdk/gen/lockboxpayload"
	sdklockbox "github.com/yandex-cloud/go-sdk/gen/lockboxsecret"
	"github.com/yandex-cloud/go-sdk/operation"
)

// secretIDPattern matches Yandex Cloud Lockbox secret IDs (20 lowercase
// alphanumeric characters, e.g. "e6q0a0ac8an7eb3fm6bu"). Anything that
// doesn't match is treated as a human-readable name and resolved via
// ListSecrets — Lockbox itself only accepts IDs in Get/Delete/AddVersion.
var secretIDPattern = regexp.MustCompile(`^[a-z0-9]{20}$`)

type Backend struct {
	client   *Client
	secrets  *sdklockbox.SecretServiceClient
	payloads *payloadsvc.PayloadServiceClient
}

func NewBackend(cfg map[string]interface{}) (backend.Backend, error) {
	folderID, _ := cfg["folder_id"].(string)
	if folderID == "" {
		return nil, errors.New("folder_id is required")
	}

	authCfg := AuthConfig{
		Type: "auto",
	}

	if auth, ok := cfg["auth"].(map[string]interface{}); ok {
		if t, ok := auth["type"].(string); ok {
			authCfg.Type = t
		}
		if token, ok := auth["token"].(string); ok {
			authCfg.Token = token
		}
		if sf, ok := auth["service_account_file"].(string); ok {
			authCfg.ServiceAccountFile = sf
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := NewClient(ctx, folderID, authCfg)
	if err != nil {
		return nil, fmt.Errorf("create lockbox client: %w", err)
	}

	secrets := client.SDK().LockboxSecret().Secret()
	payloads := client.SDK().LockboxPayload().Payload()

	return &Backend{
		client:   client,
		secrets:  secrets,
		payloads: payloads,
	}, nil
}

func (b *Backend) ListSecrets(ctx context.Context) ([]backend.Secret, error) {
	req := &lockboxpb.ListSecretsRequest{
		FolderId: b.client.FolderID(),
	}

	resp, err := b.secrets.List(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	secrets := make([]backend.Secret, 0, len(resp.GetSecrets()))
	for _, s := range resp.GetSecrets() {
		secrets = append(secrets, mapSecret(s))
	}

	return secrets, nil
}

func (b *Backend) GetSecret(ctx context.Context, nameOrID string) (*backend.Secret, error) {
	id, err := b.resolveID(ctx, nameOrID)
	if err != nil {
		return nil, err
	}
	resp, err := b.secrets.Get(ctx, &lockboxpb.GetSecretRequest{SecretId: id})
	if err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}
	secret := mapSecret(resp)
	return &secret, nil
}

func (b *Backend) GetPayload(ctx context.Context, nameOrID string, versionID string) (*backend.Payload, error) {
	id, err := b.resolveID(ctx, nameOrID)
	if err != nil {
		return nil, err
	}
	req := &lockboxpb.GetPayloadRequest{SecretId: id}
	if versionID != "" {
		req.VersionId = versionID
	}
	resp, err := b.payloads.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get payload: %w", err)
	}
	return mapPayload(id, resp), nil
}

func (b *Backend) CreateSecret(ctx context.Context, name, description string, entries []backend.Entry) (*backend.Secret, error) {
	req := &lockboxpb.CreateSecretRequest{
		FolderId:              b.client.FolderID(),
		Name:                  name,
		Description:           description,
		Labels:                map[string]string{},
		VersionPayloadEntries: toPayloadEntryChanges(entries),
	}

	op, err := b.secrets.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create secret: %w", err)
	}
	if err := operation.New(b.client.SDK().Operation(), op).Wait(ctx); err != nil {
		return nil, fmt.Errorf("create secret operation: %w", err)
	}
	meta, err := operation.New(b.client.SDK().Operation(), op).Metadata()
	if err != nil {
		return nil, fmt.Errorf("get operation metadata: %w", err)
	}
	secretMetadata, ok := meta.(*lockboxpb.CreateSecretMetadata)
	if !ok {
		return nil, fmt.Errorf("unexpected metadata type: %T", meta)
	}
	secretID := secretMetadata.GetSecretId()

	secret, err := b.secrets.Get(ctx, &lockboxpb.GetSecretRequest{SecretId: secretID})
	if err != nil {
		return nil, fmt.Errorf("get created secret: %w", err)
	}
	s := mapSecret(secret)
	return &s, nil
}

func (b *Backend) AddVersion(ctx context.Context, nameOrID string, entries []backend.Entry) error {
	id, err := b.resolveID(ctx, nameOrID)
	if err != nil {
		return err
	}
	req := &lockboxpb.AddVersionRequest{
		SecretId:       id,
		PayloadEntries: toPayloadEntryChanges(entries),
	}
	op, err := b.secrets.AddVersion(ctx, req)
	if err != nil {
		return fmt.Errorf("add version: %w", err)
	}
	if err := operation.New(b.client.SDK().Operation(), op).Wait(ctx); err != nil {
		return fmt.Errorf("add version operation: %w", err)
	}
	return nil
}

func (b *Backend) DeleteSecret(ctx context.Context, nameOrID string) error {
	id, err := b.resolveID(ctx, nameOrID)
	if err != nil {
		return err
	}
	op, err := b.secrets.Delete(ctx, &lockboxpb.DeleteSecretRequest{SecretId: id})
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	if err := operation.New(b.client.SDK().Operation(), op).Wait(ctx); err != nil {
		return fmt.Errorf("delete secret operation: %w", err)
	}
	return nil
}

// resolveID maps either a Lockbox secret ID or a human-readable name to the
// real secret ID. Lockbox APIs only accept IDs, so we list-and-match by
// name when the input doesn't look like an ID. If multiple secrets in the
// folder share the name we report it explicitly — Lockbox does allow it.
func (b *Backend) resolveID(ctx context.Context, nameOrID string) (string, error) {
	if secretIDPattern.MatchString(nameOrID) {
		return nameOrID, nil
	}
	resp, err := b.secrets.List(ctx, &lockboxpb.ListSecretsRequest{FolderId: b.client.FolderID()})
	if err != nil {
		return "", fmt.Errorf("resolve secret %q: %w", nameOrID, err)
	}
	var matches []string
	for _, s := range resp.GetSecrets() {
		if s.GetName() == nameOrID {
			matches = append(matches, s.GetId())
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("secret %q not found in folder", nameOrID)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous secret name %q: %d matches (%v) — pass an ID instead",
			nameOrID, len(matches), matches)
	}
}

// toPayloadEntryChanges converts the cross-backend Entry slice into the
// Lockbox proto type. We pick TextValue for valid UTF-8 (so values stay
// human-readable in the cloud console) and fall back to BinaryValue for
// raw bytes. Empty entry slice -> nil so the SDK omits the field.
func toPayloadEntryChanges(entries []backend.Entry) []*lockboxpb.PayloadEntryChange {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*lockboxpb.PayloadEntryChange, 0, len(entries))
	for _, e := range entries {
		change := &lockboxpb.PayloadEntryChange{Key: e.Key}
		if utf8.Valid(e.Value) {
			change.Value = &lockboxpb.PayloadEntryChange_TextValue{TextValue: string(e.Value)}
		} else {
			change.Value = &lockboxpb.PayloadEntryChange_BinaryValue{BinaryValue: e.Value}
		}
		out = append(out, change)
	}
	return out
}

func mapSecret(s *lockboxpb.Secret) backend.Secret {
	createdAt := ""
	if s.GetCreatedAt() != nil {
		createdAt = s.GetCreatedAt().AsTime().Format(time.RFC3339)
	}

	updatedAt := ""
	if s.GetCurrentVersion() != nil && s.GetCurrentVersion().GetCreatedAt() != nil {
		updatedAt = s.GetCurrentVersion().GetCreatedAt().AsTime().Format(time.RFC3339)
	}

	return backend.Secret{
		ID:          s.GetId(),
		Name:        s.GetName(),
		Description: s.GetDescription(),
		Labels:      s.GetLabels(),
		EntryKeys:   []string{},
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func mapPayload(secretID string, p *lockboxpb.Payload) *backend.Payload {
	entries := make([]backend.Entry, 0, len(p.GetEntries()))
	for _, e := range p.GetEntries() {
		var value []byte
		if txt := e.GetTextValue(); txt != "" {
			value = []byte(txt)
		} else if bin := e.GetBinaryValue(); bin != nil {
			value = bin
		}
		entries = append(entries, backend.Entry{
			Key:   e.GetKey(),
			Value: value,
		})
	}

	return &backend.Payload{
		SecretID:  secretID,
		VersionID: p.GetVersionId(),
		Entries:   entries,
	}
}

func init() {
	backend.Register("lockbox", NewBackend)
}
