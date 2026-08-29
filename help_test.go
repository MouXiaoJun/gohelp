package gohelp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSearchReturnsTopicSnippetAndPath(t *testing.T) {
	index := &Index{Documents: []Document{
		{
			Topic:    "build",
			Kind:     "command",
			HelpPath: "go help build",
			Text:     "usage: go build [-o output]\n\n-modfile file\n\tread an alternate go.mod file.",
		},
	}}

	matches := index.Search("-modfile")
	if len(matches) != 1 {
		t.Fatalf("Search() returned %d matches, want 1", len(matches))
	}
	match := matches[0]
	if match.Topic != "build" {
		t.Errorf("match topic = %q, want %q", match.Topic, "build")
	}
	if match.HelpPath != "go help build" {
		t.Errorf("match help path = %q, want %q", match.HelpPath, "go help build")
	}
	if len(match.Snippets) != 1 || !strings.Contains(match.Snippets[0], "-modfile") {
		t.Errorf("match snippets = %#v, want a -modfile snippet", match.Snippets)
	}
}

func TestSearchRequiresAllTerms(t *testing.T) {
	index := &Index{Documents: []Document{
		{Topic: "build", HelpPath: "go help build", Text: "build packages with -race"},
		{Topic: "test", HelpPath: "go help test", Text: "test packages with -run"},
	}}

	matches := index.Search("build -race")
	if len(matches) != 1 || matches[0].Topic != "build" {
		t.Fatalf("Search() = %#v, want only build", matches)
	}
}

func TestNewIndexesCurrentGoHelp(t *testing.T) {
	index, err := New(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if index.GoVersion == "" {
		t.Error("GoVersion is empty")
	}
	for _, topic := range []string{"build", "test", "mod", "mod tidy"} {
		if !hasTopic(index.Documents, topic) {
			t.Errorf("indexed topics do not contain %q", topic)
		}
	}

	matches := index.Search("-modfile")
	if !hasMatch(matches, "build", "go help build") {
		t.Errorf("Search(-modfile) = %#v, want go help build", matches)
	}
}

func TestNewReportsCommandFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-go")
	_, err := New(context.Background(), Options{GoCommand: missing})
	if err == nil {
		t.Fatal("New() succeeded with a missing go command")
	}
	if !strings.Contains(err.Error(), missing) || !strings.Contains(err.Error(), "gohelp: run") {
		t.Errorf("error = %q, want command and gohelp context", err)
	}
}

func TestNewHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New(ctx, Options{GoCommand: "go"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("New() error = %v, want context.Canceled", err)
	}
}

func TestIndexJSONRoundTrip(t *testing.T) {
	want := &Index{
		GoCommand: "go",
		GoVersion: "go version go1.23.0 darwin/arm64",
		Documents: []Document{{
			Topic:    "build",
			Kind:     "command",
			Summary:  "compile packages",
			HelpPath: "go help build",
			Text:     "usage: go build",
		}},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Index
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 1 || got.Documents[0] != want.Documents[0] || got.GoVersion != want.GoVersion {
		t.Errorf("JSON round trip = %#v, want %#v", got, *want)
	}
}

func TestIndexSaveLoad(t *testing.T) {
	want := &Index{
		GoCommand: "go",
		GoVersion: "go version go1.23.0 darwin/arm64",
		Documents: []Document{{
			Topic:    "build",
			Kind:     "command",
			HelpPath: "go help build",
			Text:     "usage: go build",
		}},
	}
	path := filepath.Join(t.TempDir(), "index.json")
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("cache permissions = %o, want 600", got)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %#v, want %#v", got, want)
	}
	if matches := got.Search("build"); len(matches) != 1 || matches[0].HelpPath != "go help build" {
		t.Errorf("loaded Search() = %#v, want build match", matches)
	}
}

func TestLoadRejectsInvalidCache(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "malformed JSON", data: "{", want: "invalid JSON"},
		{name: "missing command", data: `{ "documents": [] }`, want: "GoCommand is required"},
		{name: "missing version", data: `{ "go_command": "go", "documents": [] }`, want: "GoVersion is required"},
		{name: "missing documents", data: `{ "go_command": "go", "go_version": "go version test" }`, want: "Documents is required"},
		{name: "missing topic", data: `{ "go_command": "go", "go_version": "go version test", "documents": [{"help_path":"go help x","text":"x"}] }`, want: "Documents[0].Topic is required"},
		{name: "missing help path", data: `{ "go_command": "go", "go_version": "go version test", "documents": [{"topic":"x","text":"x"}] }`, want: "Documents[0].HelpPath is required"},
		{name: "missing text", data: `{ "go_command": "go", "go_version": "go version test", "documents": [{"topic":"x","help_path":"go help x"}] }`, want: "Documents[0].Text is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.json")
			if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func hasTopic(documents []Document, topic string) bool {
	for _, document := range documents {
		if document.Topic == topic {
			return true
		}
	}
	return false
}

func hasMatch(matches []Match, topic, helpPath string) bool {
	for _, match := range matches {
		if match.Topic == topic && match.HelpPath == helpPath {
			return true
		}
	}
	return false
}
