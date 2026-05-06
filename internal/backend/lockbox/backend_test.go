/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package lockbox

import (
	"testing"
	"time"

	lockboxpb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/jtprogru/jtsekret/internal/backend"
)

func TestSecretIDPattern(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"e6q0a0ac8an7eb3fm6bu", true},  // realistic ID
		{"a1b2c3d4e5f6g7h8i9j0", true},  // any 20 lowercase alnum
		{"E6Q0A0AC8AN7EB3FM6BU", false}, // uppercase rejected
		{"e6q0a0ac8an7eb3fm6b", false},  // 19 chars
		{"e6q0a0ac8an7eb3fm6bux", false}, // 21 chars
		{"my-secret-name", false},       // hyphens
		{"", false},
	}
	for _, c := range cases {
		got := secretIDPattern.MatchString(c.in)
		if got != c.want {
			t.Errorf("secretIDPattern.MatchString(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestToPayloadEntryChanges_Empty(t *testing.T) {
	if got := toPayloadEntryChanges(nil); got != nil {
		t.Fatalf("nil entries -> %+v, want nil", got)
	}
	if got := toPayloadEntryChanges([]backend.Entry{}); got != nil {
		t.Fatalf("empty entries -> %+v, want nil", got)
	}
}

func TestToPayloadEntryChanges_TextVsBinary(t *testing.T) {
	entries := []backend.Entry{
		{Key: "user", Value: []byte("alice")},                // valid UTF-8 -> TextValue
		{Key: "blob", Value: []byte{0xff, 0x00, 0xfe, 0xc3}}, // invalid UTF-8 -> BinaryValue
	}
	out := toPayloadEntryChanges(entries)
	if len(out) != 2 {
		t.Fatalf("got %d entries, want 2", len(out))
	}
	if got := out[0].GetKey(); got != "user" {
		t.Fatalf("entry[0].Key = %q", got)
	}
	if got := out[0].GetTextValue(); got != "alice" {
		t.Fatalf("entry[0].TextValue = %q", got)
	}
	if got := out[1].GetKey(); got != "blob" {
		t.Fatalf("entry[1].Key = %q", got)
	}
	if got := out[1].GetBinaryValue(); string(got) != string([]byte{0xff, 0x00, 0xfe, 0xc3}) {
		t.Fatalf("entry[1].BinaryValue = %v", got)
	}
	// Binary side picks BinaryValue, not TextValue.
	if out[1].GetTextValue() != "" {
		t.Fatalf("binary entry shouldn't set TextValue")
	}
}

func TestMapSecret(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 2, 2, 13, 0, 0, 0, time.UTC)
	pb := &lockboxpb.Secret{
		Id:          "e6q0a0ac8an7eb3fm6bu",
		Name:        "demo",
		Description: "test secret",
		Labels:      map[string]string{"env": "personal"},
		CreatedAt:   timestamppb.New(created),
		CurrentVersion: &lockboxpb.Version{
			CreatedAt: timestamppb.New(updated),
		},
	}
	got := mapSecret(pb)
	if got.ID != "e6q0a0ac8an7eb3fm6bu" || got.Name != "demo" {
		t.Fatalf("id/name: %+v", got)
	}
	if got.Description != "test secret" {
		t.Fatalf("description: %q", got.Description)
	}
	if got.Labels["env"] != "personal" {
		t.Fatalf("labels: %+v", got.Labels)
	}
	if got.CreatedAt != "2026-01-01T12:00:00Z" {
		t.Fatalf("createdAt: %q", got.CreatedAt)
	}
	if got.UpdatedAt != "2026-02-02T13:00:00Z" {
		t.Fatalf("updatedAt: %q", got.UpdatedAt)
	}
	if len(got.EntryKeys) != 0 {
		t.Fatalf("entry keys should be empty (lockbox List doesn't return them): %+v", got.EntryKeys)
	}
}

func TestMapSecret_NilTimestamps(t *testing.T) {
	pb := &lockboxpb.Secret{Id: "x", Name: "y"}
	got := mapSecret(pb)
	if got.CreatedAt != "" || got.UpdatedAt != "" {
		t.Fatalf("expected blank timestamps, got %+v", got)
	}
}

func TestMapPayload(t *testing.T) {
	pb := &lockboxpb.Payload{
		VersionId: "v1",
		Entries: []*lockboxpb.Payload_Entry{
			{Key: "user", Value: &lockboxpb.Payload_Entry_TextValue{TextValue: "alice"}},
			{Key: "blob", Value: &lockboxpb.Payload_Entry_BinaryValue{BinaryValue: []byte{0x01, 0x02}}},
		},
	}
	got := mapPayload("sec-id", pb)
	if got.SecretID != "sec-id" || got.VersionID != "v1" {
		t.Fatalf("ids: %+v", got)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries: %+v", got.Entries)
	}
	byKey := map[string]string{}
	for _, e := range got.Entries {
		byKey[e.Key] = string(e.Value)
	}
	if byKey["user"] != "alice" {
		t.Fatalf("user entry: %+v", got.Entries)
	}
	if byKey["blob"] != string([]byte{0x01, 0x02}) {
		t.Fatalf("blob entry: %+v", got.Entries)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"a", "b"}, "a"},
		{[]string{"", "b", "c"}, "b"},
		{[]string{"", "", ""}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := firstNonEmpty(c.in...); got != c.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
