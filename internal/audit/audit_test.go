/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>
*/
package audit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendAndLastN(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "audit.log")
	t.Setenv("JTSEKRET_AUDIT_LOG", logFile)

	for range 5 {
		err := Append(Entry{Action: "get", Secret: "s", Key: "k"})
		if err != nil {
			t.Fatal(err)
		}
	}
	es, err := LastN(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 3 {
		t.Fatalf("LastN(3) returned %d entries", len(es))
	}
	for _, e := range es {
		if e.Action != "get" || e.Secret != "s" || e.Key != "k" || e.Result != "ok" {
			t.Fatalf("unexpected entry: %+v", e)
		}
	}
}

func TestFromError(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "audit.log")
	t.Setenv("JTSEKRET_AUDIT_LOG", logFile)

	if err := FromError("delete", "file", "x", "", errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if err := FromError("delete", "file", "y", "", nil); err != nil {
		t.Fatal(err)
	}
	es, _ := LastN(0)
	if len(es) != 2 {
		t.Fatalf("got %d entries", len(es))
	}
	if es[0].Result != "error" || es[0].Error != "boom" {
		t.Fatalf("entry 0: %+v", es[0])
	}
	if es[1].Result != "ok" || es[1].Error != "" {
		t.Fatalf("entry 1: %+v", es[1])
	}
}

func TestLastN_FileMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("JTSEKRET_AUDIT_LOG", filepath.Join(tmp, "nope.log"))
	es, err := LastN(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 0 {
		t.Fatal("expected empty slice for missing log")
	}
}

func TestPath_HonoursOverride(t *testing.T) {
	t.Setenv("JTSEKRET_AUDIT_LOG", "/tmp/x.log")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/tmp/x.log" {
		t.Fatalf("got %q", p)
	}
}

func TestDisable(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "audit.log")
	t.Setenv("JTSEKRET_AUDIT_LOG", logFile)

	t.Cleanup(func() {
		mu.Lock()
		disabled = false
		mu.Unlock()
	})
	Disable()
	if err := Append(Entry{Action: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("log written despite Disable(): %v", err)
	}
}
