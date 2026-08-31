package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MouXiaoJun/gohelp"
)

func TestRunUsesValidCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	index := &gohelp.Index{
		GoCommand: "go",
		GoVersion: currentGoVersion(t),
		Documents: []gohelp.Document{{
			Topic:    "cache-only-topic",
			HelpPath: "go help cache-only-topic",
			Text:     "cache-only result",
		}},
	}
	if err := index.Save(path); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cache", path, "-q", "cache-only-topic"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "go help cache-only-topic") {
		t.Fatalf("run() output = %q, want cached help path", stdout.String())
	}
}

func TestRunPrintsOptionExplanation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	index := &gohelp.Index{
		GoCommand: "go",
		GoVersion: currentGoVersion(t),
		Documents: []gohelp.Document{{
			Topic: "testflag", HelpPath: "go help testflag",
			Text: "-bench regexp\n    Run matching benchmarks.\n-benchtime t\n    Run for a duration.",
		}},
	}
	if err := index.Save(path); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-cache", path, "-q=-bench"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d, stderr %s", code, &stderr)
	}
	want := "testflag\n  go help testflag\n    -bench regexp\n        Run matching benchmarks.\n"
	if stdout.String() != want {
		t.Fatalf("output = %q, want %q", stdout.String(), want)
	}
}

func TestRunRebuildsCacheWhenGoVersionChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	index := &gohelp.Index{
		GoCommand: "go",
		GoVersion: "go version old-toolchain",
		Documents: []gohelp.Document{{
			Topic:    "cache-only-topic",
			HelpPath: "go help cache-only-topic",
			Text:     "cache-only result",
		}},
	}
	if err := index.Save(path); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cache", path, "-q", "cache-only-topic"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No help matches") {
		t.Fatalf("run() output = %q, want rebuilt index without stale result", stdout.String())
	}
	got, err := gohelp.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.GoVersion != currentGoVersion(t) {
		t.Errorf("rebuilt GoVersion = %q, want current version", got.GoVersion)
	}
}

func TestRunRefreshesValidCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	index := &gohelp.Index{
		GoCommand: "go",
		GoVersion: currentGoVersion(t),
		Documents: []gohelp.Document{{
			Topic:    "cache-only-topic",
			HelpPath: "go help cache-only-topic",
			Text:     "cache-only result",
		}},
	}
	if err := index.Save(path); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cache", path, "-refresh", "-q", "cache-only-topic"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No help matches") {
		t.Fatalf("run() output = %q, want refreshed index without stale result", stdout.String())
	}
}

func TestRunRebuildsWhenCachedCommandDiffers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	index := &gohelp.Index{
		GoCommand: "go",
		GoVersion: currentGoVersion(t),
		Documents: []gohelp.Document{{
			Topic:    "cache-only-topic",
			HelpPath: "go help cache-only-topic",
			Text:     "cache-only result",
		}},
	}
	if err := index.Save(path); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cache", path, "-go", "missing-go-command", "-q", "cache-only-topic"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing-go-command") || !strings.Contains(stderr.String(), "gohelp: run") {
		t.Fatalf("stderr = %q, want clear go command error", stderr.String())
	}
}

func currentGoVersion(t *testing.T) string {
	t.Helper()
	version, err := gohelp.Version(context.Background(), gohelp.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return version
}
