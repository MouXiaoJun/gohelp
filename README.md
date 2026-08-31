# gohelp

`gohelp` is a small, standard-library-only Go package and CLI for searching the help exposed by the installed `go` command.

It does not embed a copy of Go documentation. `New` runs `go version`, `go help`, and the discovered `go help <topic>` paths, so the index follows the local toolchain.

## Library

```go
ctx := context.Background()
index, err := gohelp.New(ctx, gohelp.Options{})
if err != nil {
	log.Fatal(err)
}

for _, match := range index.Search("-modfile") {
	fmt.Println(match.Topic, match.HelpPath)
	for _, snippet := range match.Snippets {
		fmt.Println(snippet)
	}
}
```

`Search` treats whitespace-separated terms as a case-insensitive AND query.
Terms starting with `-` match option boundaries: `-bench` matches `-bench regexp`
and `'-bench=.'`, but not `-benchtime`, `-benchmem` or `--bench`. Letters, digits,
underscores and hyphens adjoining an option are not boundaries. Ordinary words
retain substring matching. Ranking uses the same rule as filtering and snippets.

A `Match` contains the topic, up to five matching snippets, and a suggested path
such as `go help build`. A snippet includes its immediately following, more
deeply indented explanation, including paragraphs, and stops before a peer option
or section. Snippets may contain newlines; this relies on Go help indentation,
not a general documentation parser. `Index`, `Document`, and `Match` have JSON
tags and can be passed to `encoding/json` directly.

`Options.GoCommand` can select another executable; it defaults to `go`. Commands are run with the supplied `context.Context`, and failures include the command and command output.

Use `Save` and `Load` to persist an index as readable JSON:

```go
if err := index.Save("gohelp-cache.json"); err != nil {
	log.Fatal(err)
}
cached, err := gohelp.Load("gohelp-cache.json")
if err != nil {
	log.Fatal(err)
}
```

`Load` rejects incomplete or malformed cache files.

`Save` now applies the same field validation before touching the cache. It writes
owner-only JSON to a temporary file in the cache directory, closes it, then replaces
the destination. Failed validation, writing, or closing leaves the previous cache
untouched; temporary files are removed on failure. The directory must be writable.
The destination entry is replaced (including a symbolic link), rather than writing
through it. Replacement is atomic on Unix; this is not guaranteed on Windows or
other non-Unix platforms, and crash/power-loss durability is not promised.

## CLI

```text
go run ./cmd/gohelp -q=-modfile
go run ./cmd/gohelp -q=-bench
go run ./cmd/gohelp -q=-benchtime
go run ./cmd/gohelp -q testflag -json
go run ./cmd/gohelp -cache gohelp-cache.json -q=-modfile
go run ./cmd/gohelp -cache gohelp-cache.json -refresh -q=-modfile
```

The CLI prints matching snippets and help paths. Use `-json` for a JSON array of matches, `-go` to select the executable, `-cache` to load or refresh an index cache, and `-refresh` to force rebuilding it. Without `-cache`, the CLI keeps rebuilding the index as before.

## Development

Requires Go 1.23+. CI checks Go 1.23.0 and the current stable release on Ubuntu.
Tests use both fixed boundary samples and the actual installed `go help` output;
the Go command must be on PATH. Maintenance stays focused on local-toolchain
help search and cache correctness, without embedded documentation or remote AI.

```text
export GOWORK=off
gofmt -l .
go build ./...
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
```
