// Package gohelp indexes the help text produced by the installed go command.
package gohelp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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

// Save writes the index as readable JSON with owner-only permissions.
func (index *Index) Save(path string) error {
	if index == nil {
		return fmt.Errorf("gohelp: save index %q: nil index", path)
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("gohelp: save index %q: encode JSON: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("gohelp: save index %q: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("gohelp: save index %q: set permissions: %w", path, err)
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

// Match is a search result with lines from the matching help document.
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
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
			score += strings.Count(haystack, term)
			if strings.Contains(topic, term) {
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
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		matched := false
		for _, term := range terms {
			if strings.Contains(lower, term) {
				matched = true
				break
			}
		}
		line = strings.TrimSpace(line)
		if !matched || line == "" || seen[line] {
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
