/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jtprogru/jtsekret/internal/domain"
)

func TestNewOutputter_Selects(t *testing.T) {
	cases := []struct {
		fmt  Format
		want string
	}{
		{FormatJSON, "JSON"},
		{FormatTable, "Table"},
		{FormatPlain, "Plain"},
		{FormatAuto, "Plain"}, // auto / unknown -> plain default
		{Format("xyz"), "Plain"},
	}
	for _, c := range cases {
		if got := typeName(NewOutputter(c.fmt)); got != c.want {
			t.Fatalf("NewOutputter(%q): got %q, want %q", c.fmt, got, c.want)
		}
	}
}

// typeName collapses concrete Outputter implementations to a short label
// so tests don't have to import reflect.
func typeName(o Outputter) string {
	switch o.(type) {
	case *PlainOutputter:
		return "Plain"
	case *TableOutputter:
		return "Table"
	case *JSONOutputter:
		return "JSON"
	default:
		return "?"
	}
}

func sampleSecret() domain.Secret {
	return domain.Secret{
		ID: "id-1", Name: "demo", Description: "test", Labels: nil,
		EntryKeys: []string{"a", "b"},
	}
}
func samplePayload() domain.Payload {
	return domain.Payload{
		SecretID:  "id-1",
		VersionID: "1",
		Entries: []domain.Entry{
			{Key: "a", Value: []byte("v1")},
			{Key: "b", Value: []byte("v2")},
		},
	}
}

func TestPlainOutputter(t *testing.T) {
	p := &PlainOutputter{}
	var b bytes.Buffer

	s := sampleSecret()
	if err := p.PrintSecret(&b, &s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Name: demo") || !strings.Contains(b.String(), "ID: id-1") {
		t.Fatalf("PrintSecret: %q", b.String())
	}

	b.Reset()
	if err := p.PrintSecretList(&b, []domain.Secret{s}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "demo\tid-1") {
		t.Fatalf("PrintSecretList: %q", b.String())
	}

	b.Reset()
	pl := samplePayload()
	if err := p.PrintPayload(&b, &pl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "a=v1") || !strings.Contains(b.String(), "b=v2") {
		t.Fatalf("PrintPayload: %q", b.String())
	}

	b.Reset()
	if err := p.PrintEntry(&b, "k", []byte("V")); err != nil {
		t.Fatal(err)
	}
	if b.String() != "V" {
		t.Fatalf("PrintEntry: %q", b.String())
	}

	b.Reset()
	if err := p.PrintMessage(&b, "hi"); err != nil {
		t.Fatal(err)
	}
	if b.String() != "hi\n" {
		t.Fatalf("PrintMessage: %q", b.String())
	}
}

func TestTableOutputter(t *testing.T) {
	to := &TableOutputter{}
	var b bytes.Buffer

	s := sampleSecret()
	if err := to.PrintSecretList(&b, []domain.Secret{s}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "ID") || !strings.Contains(out, "demo") {
		t.Fatalf("PrintSecretList: %q", out)
	}

	// Description longer than 50 chars gets ellipsised.
	long := s
	long.Description = strings.Repeat("x", 80)
	b.Reset()
	if err := to.PrintSecretList(&b, []domain.Secret{long}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "...") {
		t.Fatalf("expected ellipsis on >50-char description: %q", b.String())
	}

	b.Reset()
	pl := samplePayload()
	if err := to.PrintPayload(&b, &pl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "KEY") || !strings.Contains(b.String(), "v1") {
		t.Fatalf("PrintPayload: %q", b.String())
	}

	b.Reset()
	if err := to.PrintSecret(&b, &s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Name:") || !strings.Contains(b.String(), "demo") {
		t.Fatalf("PrintSecret: %q", b.String())
	}

	b.Reset()
	_ = to.PrintEntry(&b, "k", []byte("V"))
	if b.String() != "V" {
		t.Fatalf("PrintEntry: %q", b.String())
	}

	b.Reset()
	_ = to.PrintMessage(&b, "hi")
	if b.String() != "hi\n" {
		t.Fatalf("PrintMessage: %q", b.String())
	}
}

func TestJSONOutputter(t *testing.T) {
	j := &JSONOutputter{}
	var b bytes.Buffer

	s := sampleSecret()
	if err := j.PrintSecret(&b, &s); err != nil {
		t.Fatal(err)
	}
	var got domain.Secret
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("PrintSecret: not JSON: %v / %q", err, b.String())
	}
	if got.Name != "demo" {
		t.Fatalf("PrintSecret roundtrip: %+v", got)
	}

	b.Reset()
	if err := j.PrintSecretList(&b, []domain.Secret{s}); err != nil {
		t.Fatal(err)
	}
	var listed []domain.Secret
	if err := json.Unmarshal(b.Bytes(), &listed); err != nil {
		t.Fatalf("PrintSecretList: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "demo" {
		t.Fatalf("PrintSecretList roundtrip: %+v", listed)
	}

	b.Reset()
	pl := samplePayload()
	if err := j.PrintPayload(&b, &pl); err != nil {
		t.Fatal(err)
	}
	var entries []map[string]string
	if err := json.Unmarshal(b.Bytes(), &entries); err != nil {
		t.Fatalf("PrintPayload: %v", err)
	}
	if len(entries) != 2 || entries[0]["key"] != "a" || entries[0]["value"] != "v1" {
		t.Fatalf("PrintPayload roundtrip: %+v", entries)
	}

	b.Reset()
	if err := j.PrintEntry(&b, "k", []byte("V")); err != nil {
		t.Fatal(err)
	}
	var entry map[string]string
	if err := json.Unmarshal(b.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["k"] != "V" {
		t.Fatalf("PrintEntry roundtrip: %+v", entry)
	}

	b.Reset()
	if err := j.PrintMessage(&b, "hi"); err != nil {
		t.Fatal(err)
	}
	var msg map[string]string
	if err := json.Unmarshal(b.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if msg["message"] != "hi" {
		t.Fatalf("PrintMessage roundtrip: %+v", msg)
	}
}

func TestDetectFormat_NonTTYReturnsPlain(t *testing.T) {
	// In `go test` stdout is a pipe, never a TTY.
	if got := DetectFormat(); got != FormatPlain {
		t.Fatalf("DetectFormat under go test: got %q, want %q", got, FormatPlain)
	}
}
