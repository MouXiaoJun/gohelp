// Package gohelp indexes the help text produced by the installed go command.
package gohelp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Options controls how New invokes the go command.
type Options struct {
	// GoCommand is the executable to run. It defaults to "go".
	GoCommand string
}

// Index contains help text from one go toolchain.
type Index struct {
	GoCommand string     `json:"go_command"`
	GoVersion string     `json:"go_version"`
	Documents []Document `json:"documents"`
}

// Document is one go help topic or command.
type Document struct {
	Topic    string `json:"topic"`
	Kind     string `json:"kind"`
	Summary  string `json:"summary,omitempty"`
	HelpPath string `json:"help_path"`
	Text     string `json:"text"`
}

// Save validates the index and replaces path with readable JSON with owner-only
// permissions. It writes and closes a temporary file in the same directory before
// renaming it over path. Atomic replacement is not guaranteed on non-Unix systems.
// Save does not guarantee durability across a crash or power loss.
func (index *Index) Save(path string) error {
	if index == nil {
		return fmt.Errorf("gohelp: save index %q: nil index", path)
	}
	if err := index.validate(); err != nil {
		return fmt.Errorf("gohelp: save index %q: invalid index: %w", path, err)
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("gohelp: save index %q: encode JSON: %w", path, err)
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".gohelp-*.json")
	if err != nil {
		return fmt.Errorf("gohelp: save index %q: create temporary file: %w", path, err)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("gohelp: save index %q: write temporary file: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("gohelp: save index %q: close temporary file: %w", path, closeErr)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("gohelp: save index %q: replace cache: %w", path, err)
	}
	return nil
}

// Load reads and validates an index saved as JSON.
func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gohelp: load index %q: %w", path, err)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("gohelp: invalid index %q: invalid JSON: %w", path, err)
	}
	if err := index.validate(); err != nil {
		return nil, fmt.Errorf("gohelp: invalid index %q: %w", path, err)
	}
	return &index, nil
}

func (index *Index) validate() error {
	if strings.TrimSpace(index.GoCommand) == "" {
		return fmt.Errorf("GoCommand is required")
	}
	if strings.TrimSpace(index.GoVersion) == "" {
		return fmt.Errorf("GoVersion is required")
	}
	if index.Documents == nil {
		return fmt.Errorf("Documents is required")
	}
	for i, document := range index.Documents {
		if strings.TrimSpace(document.Topic) == "" {
			return fmt.Errorf("Documents[%d].Topic is required", i)
		}
		if strings.TrimSpace(document.HelpPath) == "" {
			return fmt.Errorf("Documents[%d].HelpPath is required", i)
		}
		if strings.TrimSpace(document.Text) == "" {
			return fmt.Errorf("Documents[%d].Text is required", i)
		}
	}
	return nil
}

// Match is a search result with snippets from the matching help document.
type Match struct {
	Topic    string   `json:"topic"`
	Snippets []string `json:"snippets"`
	HelpPath string   `json:"help_path"`
}

type entry struct {
	args    []string
	kind    string
	summary string
}

// Version returns the version reported by the configured go command.
func Version(ctx context.Context, options Options) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("gohelp: nil context")
	}
	command := options.GoCommand
	if command == "" {
		command = "go"
	}
	version, err := run(ctx, command, "version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(version), nil
}

// New builds an index from the currently installed go command.
func New(ctx context.Context, options Options) (*Index, error) {
	if ctx == nil {
		return nil, fmt.Errorf("gohelp: nil context")
	}
	command := options.GoCommand
	if command == "" {
		command = "go"
	}

	version, err := Version(ctx, options)
	if err != nil {
		return nil, err
	}
	root, err := run(ctx, command, "help")
	if err != nil {
		return nil, err
	}

	index := &Index{
		GoCommand: command,
		GoVersion: version,
		Documents: []Document{{
			Topic:    "go",
			Kind:     "overview",
			HelpPath: "go help",
			Text:     root,
		}},
	}
	seen := map[string]bool{"go": true}
	queue := make([]entry, 0)
	for _, item := range parseEntries(root, "The commands are:") {
		queue = append(queue, entry{args: []string{item.name}, kind: "command", summary: item.summary})
	}
	for _, item := range parseEntries(root, "Additional help topics:") {
		queue = append(queue, entry{args: []string{item.name}, kind: "topic", summary: item.summary})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		topic := strings.Join(item.args, " ")
		if seen[topic] {
			continue
		}
		seen[topic] = true

		text, err := run(ctx, command, append([]string{"help"}, item.args...)...)
		if err != nil {
			return nil, err
		}
		index.Documents = append(index.Documents, Document{
			Topic:    topic,
			Kind:     item.kind,
			Summary:  item.summary,
			HelpPath: "go help " + topic,
			Text:     text,
		})

		for _, child := range parseEntries(text, "The commands are:") {
			args := append(append([]string(nil), item.args...), child.name)
			queue = append(queue, entry{args: args, kind: "command", summary: child.summary})
		}
	}

	return index, nil
}

// Search returns documents containing every whitespace-separated query term.
// Matching is case-insensitive. Terms starting with '-' match option boundaries;
// other terms match substrings. Snippets include adjacent indented explanations.
// Results are ranked by the number of term occurrences, with topic matches first.
func (index *Index) Search(query string) []Match {
	if index == nil {
		return nil
	}
	terms := uniqueTerms(query)
	if len(terms) == 0 {
		return nil
	}

	type rankedMatch struct {
		Match
		score int
	}
	ranked := make([]rankedMatch, 0)
	for _, document := range index.Documents {
		topic := strings.ToLower(document.Topic)
		text := strings.ToLower(document.Text)
		haystack := topic + "\n" + text
		score := 0
		matches := true
		for _, term := range terms {
			count := countTerm(haystack, term)
			if count == 0 {
				matches = false
				break
			}
			score += count
			if countTerm(topic, term) > 0 {
				score += 100
			}
		}
		if !matches {
			continue
		}
		ranked = append(ranked, rankedMatch{
			Match: Match{
				Topic:    document.Topic,
				Snippets: snippets(document.Text, terms),
				HelpPath: document.HelpPath,
			},
			score: score,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].Topic < ranked[j].Topic
	})
	results := make([]Match, len(ranked))
	for i := range ranked {
		results[i] = ranked[i].Match
	}
	return results
}

type parsedEntry struct {
	name    string
	summary string
}

func parseEntries(text, heading string) []parsedEntry {
	lines := strings.Split(text, "\n")
	entries := make([]parsedEntry, 0)
	inSection := false
	for _, line := range lines {
		if !inSection {
			if strings.TrimSpace(line) == heading {
				inSection = true
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] != '\t' && line[0] != ' ' {
			break
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		remainder := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
		entries = append(entries, parsedEntry{name: fields[0], summary: remainder})
	}
	return entries
}

func uniqueTerms(query string) []string {
	seen := make(map[string]bool)
	terms := make([]string, 0)
	for _, field := range strings.Fields(strings.ToLower(query)) {
		if !seen[field] {
			seen[field] = true
			terms = append(terms, field)
		}
	}
	return terms
}

func snippets(text string, terms []string) []string {
	result := make([]string, 0, 5)
	seen := make(map[string]bool)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		lower := strings.ToLower(line)
		matched := false
		for _, term := range terms {
			if countTerm(lower, term) > 0 {
				matched = true
				break
			}
		}
		if !matched || strings.TrimSpace(line) == "" {
			continue
		}
		// ponytail: use Go help's indentation, not a general document parser;
		// a help format change needs fixture updates. Stop at peer sections.
		end := i + 1
		for next := end; next < len(lines); next++ {
			if strings.TrimSpace(lines[next]) == "" {
				continue
			}
			if indentation(lines[next]) <= indentation(line) {
				break
			}
			end = next + 1
		}
		line = strings.TrimSpace(strings.Join(lines[i:end], "\n"))
		i = end - 1
		if seen[line] {
			continue
		}
		seen[line] = true
		result = append(result, line)
		if len(result) == cap(result) {
			break
		}
	}
	if len(result) == 0 {
		for _, line := range strings.Split(text, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				result = append(result, line)
				break
			}
		}
	}
	return result
}

// countTerm receives lower-case text and terms, just like Search and snippets.
func countTerm(text, term string) int {
	if !strings.HasPrefix(term, "-") {
		return strings.Count(text, term)
	}
	count := 0
	for offset := 0; offset < len(text); {
		i := strings.Index(text[offset:], term)
		if i < 0 {
			break
		}
		i += offset
		end := i + len(term)
		before, _ := utf8.DecodeLastRuneInString(text[:i])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if !optionRune(before) && !optionRune(after) {
			count++
		}
		offset = end
	}
	return count
}

func optionRune(r rune) bool {
	return r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func indentation(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 8 - width%8
		default:
			return width
		}
	}
	return width
}

func run(ctx context.Context, command string, args ...string) (string, error) {
	commandLine := strings.Join(append([]string{command}, args...), " ")
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("gohelp: run %s: %w", commandLine, err)
	}
	output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("gohelp: run %s: %w", commandLine, ctxErr)
	}
	detail := strings.TrimSpace(string(output))
	if detail != "" {
		return "", fmt.Errorf("gohelp: run %s: %w: %s", commandLine, err, detail)
	}
	return "", fmt.Errorf("gohelp: run %s: %w", commandLine, err)
}
