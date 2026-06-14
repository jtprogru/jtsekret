/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jtprogru/jtsekret/internal/backend"
)

func TestValidateEntryKey(t *testing.T) {
	valid := []string{"id_rsa", "config.json", "key-1", "a.b.c"}
	for _, k := range valid {
		if err := validateEntryKey(k); err != nil {
			t.Errorf("validateEntryKey(%q) = %v, want nil", k, err)
		}
	}

	invalid := []string{
		"",
		"..",
		"../x",
		"../../etc/passwd",
		"a/b",
		`a\b`,
		"/abs",
		"dir/..",
		"..\\win",
	}
	for _, k := range invalid {
		if err := validateEntryKey(k); err == nil {
			t.Errorf("validateEntryKey(%q) = nil, want error", k)
		}
	}
}

// setDumpFlags sets the package-global dump flags for a test and restores them.
// force is always enabled so tests never block on the overwrite prompt.
func setDumpFlags(t *testing.T, output, dir string) {
	t.Helper()
	prevOut, prevDir, prevForce := dumpOutput, dumpDir, dumpForce
	dumpOutput, dumpDir, dumpForce = output, dir, true
	t.Cleanup(func() {
		dumpOutput, dumpDir, dumpForce = prevOut, prevDir, prevForce
	})
}

func TestDumpEntry_RejectsTraversalKey(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "out")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	setDumpFlags(t, "", dir)

	// A crafted entry key that would escape --dir into the parent.
	e := backend.Entry{Key: "../evil", Value: []byte("pwned")}
	if err := dumpEntry(e, 0o600); err == nil {
		t.Fatal("dumpEntry accepted traversal key, want error")
	}

	// Nothing must have been written outside the target directory.
	if _, err := os.Stat(filepath.Join(base, "evil")); !os.IsNotExist(err) {
		t.Fatalf("traversal wrote a file outside --dir: stat err = %v", err)
	}
}

func TestDumpEntry_WritesNormalKey(t *testing.T) {
	dir := t.TempDir()
	setDumpFlags(t, "", dir)

	e := backend.Entry{Key: "id_rsa", Value: []byte("secret-bytes")}
	if err := dumpEntry(e, 0o600); err != nil {
		t.Fatalf("dumpEntry: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "id_rsa"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "secret-bytes" {
		t.Fatalf("content = %q, want %q", got, "secret-bytes")
	}
}

// With --output set to an explicit path, the entry key is irrelevant: the file
// must be written verbatim to that trusted CLI-provided path.
func TestDumpEntry_OutputPath(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "target.pem")
	setDumpFlags(t, out, "")

	// Even a key that would be rejected as a filename is ignored when --output
	// names the destination explicitly.
	e := backend.Entry{Key: "../would-be-rejected", Value: []byte("payload")}
	if err := dumpEntry(e, 0o600); err != nil {
		t.Fatalf("dumpEntry: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read --output file: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("content = %q, want %q", got, "payload")
	}
}

// --output - writes the raw value to stdout and creates no file.
func TestDumpEntry_OutputStdout(t *testing.T) {
	setDumpFlags(t, "-", "")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prevStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = prevStdout }()

	e := backend.Entry{Key: "id_rsa", Value: []byte("to-stdout")}
	dumpErr := dumpEntry(e, 0o600)
	_ = w.Close()
	os.Stdout = prevStdout

	if dumpErr != nil {
		t.Fatalf("dumpEntry: %v", dumpErr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "to-stdout" {
		t.Fatalf("stdout = %q, want %q", out, "to-stdout")
	}
}
