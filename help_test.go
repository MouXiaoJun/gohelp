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

func TestSearchOptionBoundaries(t *testing.T) {
	for _, test := range []struct {
		text string
		want bool
	}{
		{"-bench regexp", true},
		{"use '-bench=.' or (-BENCH X).", true},
		{"[-bench], -bench;", true},
		{"-benchtime t", false},
		{"-benchmem", false},
		{"--bench", false},
		{"prefix-bench", false},
		{"-bench-extra", false},
		{"-bench_extra", false},
		{"-bench2", false},
		{"-bench中文", false},
		{"中文-bench", false},
	} {
		t.Run(test.text, func(t *testing.T) {
			index := &Index{Documents: []Document{{Topic: "testflag", Text: test.text}}}
			if got := len(index.Search("-bench")) == 1; got != test.want {
				t.Fatalf("Search(-bench) in %q = %v, want %v", test.text, got, test.want)
			}
		})
	}
	index := &Index{Documents: []Document{
		{Topic: "a", Text: "-bench\n" + strings.Repeat("-benchtime ", 10)},
		{Topic: "b", Text: "-bench -bench"},
	}}
	if matches := index.Search("-bench"); len(matches) != 2 || matches[0].Topic != "b" {
		t.Fatalf("option prefixes must not affect ranking: %#v", matches)
	}
	if matches := index.Search("bencht"); len(matches) != 1 || matches[0].Topic != "a" {
		t.Fatalf("ordinary words must retain substring matching: %#v", matches)
	}
}

func TestSearchIncludesAdjacentExplanation(t *testing.T) {
	const text = "\t-bench regexp\n\t    Run matching benchmarks.\n\t    Use -bench=. for all.\n\n\t    Another paragraph.\n\t        go test -bench=.\n\n\t-benchtime t\n\t    Run for this duration.\n\nOther help.\n"
	for _, newline := range []string{"\n", "\r\n"} {
		index := &Index{Documents: []Document{{Topic: "testflag", Text: strings.ReplaceAll(text, "\n", newline)}}}
		matches := index.Search("-bench")
		want := strings.TrimSpace(strings.Split(text, "\n\n\t-benchtime")[0])
		if len(matches) != 1 || len(matches[0].Snippets) != 1 || matches[0].Snippets[0] != want {
			t.Fatalf("Search(-bench) snippets = %#v, want %q", matches, want)
		}
		matches = index.Search("-benchtime")
		if len(matches) != 1 || len(matches[0].Snippets) != 1 || matches[0].Snippets[0] != "-benchtime t\n\t    Run for this duration." {
			t.Fatalf("Search(-benchtime) snippets = %#v", matches)
		}
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
	t.Log(index.GoVersion)
	for _, flag := range []string{"-bench", "-benchtime"} {
		found := false
		for _, match := range index.Search(flag) {
			if match.Topic != "testflag" {
				continue
			}
			for _, snippet := range match.Snippets {
				if strings.HasPrefix(snippet, flag+" ") && strings.Contains(snippet, "\n") {
					found = true
				}
				if flag == "-bench" && strings.Contains(snippet, "-benchtime") {
					t.Errorf("-bench snippet includes -benchtime: %q", snippet)
				}
			}
		}
		if !found {
			t.Errorf("real go help testflag has no %s definition with explanation", flag)
		}
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

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("invalid index preserves cache", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.json")
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		missingVersion := *want
		missingVersion.GoVersion = ""
		missingText := *want
		missingText.Documents = []Document{{Topic: "build", HelpPath: "go help build"}}
		for _, invalid := range []*Index{nil, {}, &missingVersion, &missingText} {
			if err := invalid.Save(path); err == nil {
				t.Errorf("Save(%#v) succeeded, want validation error", invalid)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != string(original) {
				t.Fatalf("invalid Save changed previous cache: data=%s, err=%v", data, err)
			}
		}
	})
	t.Run("replacement preserves previous file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.json")
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		// A hard link observes the previous file even after the cache path is replaced.
		previous := path + ".previous"
		if err := os.Link(path, previous); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		updated := *want
		updated.GoVersion = "go version replacement"
		if err := updated.Save(path); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(previous)
		if err != nil || string(data) != string(original) {
			t.Errorf("Save overwrote previous file: data=%s, err=%v", data, err)
		}
		got, err := Load(path)
		if err != nil || !reflect.DeepEqual(got, &updated) {
			t.Fatalf("replacement Load() = %#v, %v", got, err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 2 {
			t.Errorf("cache directory = %v, %v; want cache and previous file only", entries, err)
		}
	})
	t.Run("failed replacement cleans up", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "cache")
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := want.Save(target); err == nil {
			t.Fatal("Save to a directory succeeded")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 || !entries[0].IsDir() {
			t.Errorf("failed Save changed target or left temporary files: %v, %v", entries, err)
		}
	})
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
