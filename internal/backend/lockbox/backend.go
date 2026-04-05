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
	"fmt"
	"time"

	"github.com/jtprogru/jtsekret/internal/backend"
	lockboxpb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
	payloadsvc "github.com/yandex-cloud/go-sdk/gen/lockboxpayload"
	sdklockbox "github.com/yandex-cloud/go-sdk/gen/lockboxsecret"
	"github.com/yandex-cloud/go-sdk/operation"
)

type LockboxBackend struct {
	client   *Client
	secrets  *sdklockbox.SecretServiceClient
	payloads *payloadsvc.PayloadServiceClient
}

func NewBackend(cfg map[string]interface{}) (backend.Backend, error) {
	folderID, _ := cfg["folder_id"].(string)
	if folderID == "" {
		return nil, fmt.Errorf("folder_id is required")
	}

	authCfg := AuthConfig{
		Type: "oauth",
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

	return &LockboxBackend{
		client:   client,
		secrets:  secrets,
		payloads: payloads,
	}, nil
}

func (b *LockboxBackend) ListSecrets(ctx context.Context) ([]backend.Secret, error) {
	req := &lockboxpb.ListSecretsRequest{
		FolderId: b.client.FolderID(),
	}

	resp, err := b.secrets.List(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	secrets := make([]backend.Secret, 0, len(resp.Secrets))
	for _, s := range resp.Secrets {
		secrets = append(secrets, mapSecret(s))
	}

	return secrets, nil
}

func (b *LockboxBackend) GetSecret(ctx context.Context, nameOrID string) (*backend.Secret, error) {
	req := &lockboxpb.GetSecretRequest{
		SecretId: nameOrID,
	}

	resp, err := b.secrets.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}

	secret := mapSecret(resp)
	return &secret, nil
}

func (b *LockboxBackend) GetPayload(ctx context.Context, nameOrID string, versionID string) (*backend.Payload, error) {
	req := &lockboxpb.GetPayloadRequest{
		SecretId: nameOrID,
	}

	if versionID != "" {
		req.VersionId = versionID
	}

	resp, err := b.payloads.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get payload: %w", err)
	}

	return mapPayload(nameOrID, resp), nil
}

func (b *LockboxBackend) CreateSecret(ctx context.Context, name, description string, entries []backend.Entry) (*backend.Secret, error) {
	req := &lockboxpb.CreateSecretRequest{
		FolderId:    b.client.FolderID(),
		Name:        name,
		Description: description,
		Labels:      map[string]string{},
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

	secret, err := b.secrets.Get(ctx, &lockboxpb.GetSecretRequest{
		SecretId: secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("get created secret: %w", err)
	}

	s := mapSecret(secret)
	return &s, nil
}

func (b *LockboxBackend) AddVersion(ctx context.Context, nameOrID string, entries []backend.Entry) error {
	req := &lockboxpb.AddVersionRequest{
		SecretId: nameOrID,
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

func (b *LockboxBackend) DeleteSecret(ctx context.Context, nameOrID string) error {
	req := &lockboxpb.DeleteSecretRequest{
		SecretId: nameOrID,
	}

	op, err := b.secrets.Delete(ctx, req)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}

	if err := operation.New(b.client.SDK().Operation(), op).Wait(ctx); err != nil {
		return fmt.Errorf("delete secret operation: %w", err)
	}

	return nil
}

func mapSecret(s *lockboxpb.Secret) backend.Secret {
	createdAt := ""
	if s.CreatedAt != nil {
		createdAt = s.CreatedAt.AsTime().Format(time.RFC3339)
	}

	updatedAt := ""
	if s.CurrentVersion != nil && s.CurrentVersion.CreatedAt != nil {
		updatedAt = s.CurrentVersion.CreatedAt.AsTime().Format(time.RFC3339)
	}

	return backend.Secret{
		ID:          s.Id,
		Name:        s.Name,
		Description: s.Description,
		Labels:      s.Labels,
		EntryKeys:   []string{},
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func mapPayload(secretID string, p *lockboxpb.Payload) *backend.Payload {
	entries := make([]backend.Entry, 0, len(p.Entries))
	for _, e := range p.Entries {
		var value []byte
		if txt := e.GetTextValue(); txt != "" {
			value = []byte(txt)
		} else if bin := e.GetBinaryValue(); bin != nil {
			value = bin
		}
		entries = append(entries, backend.Entry{
			Key:   e.Key,
			Value: value,
		})
	}

	return &backend.Payload{
		SecretID:  secretID,
		VersionID: p.VersionId,
		Entries:   entries,
	}
}

func init() {
	backend.Register("lockbox", NewBackend)
}
