package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MouXiaoJun/gohelp"
)

func TestRunUsesValidCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	index := &gohelp.Index{
		GoCommand: "go",
		Documents: []gohelp.Document{{
			Topic:    "cached",
			HelpPath: "go help cached",
			Text:     "cached-only result",
		}},
	}
	if err := index.Save(path); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cache", path, "-go", "missing-go-command", "-q", "cached"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "go help cached") {
		t.Fatalf("run() output = %q, want cached help path", stdout.String())
	}
}
