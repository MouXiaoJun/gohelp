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

`Search` treats whitespace-separated terms as an AND query. A `Match` contains the topic, matching text lines, and a suggested path such as `go help build`. `Index`, `Document`, and `Match` have JSON tags and can be passed to `encoding/json` directly.

`Options.GoCommand` can select another executable; it defaults to `go`. Commands are run with the supplied `context.Context`, and failures include the command and command output.

## CLI

```text
go run ./cmd/gohelp -q=-modfile
go run ./cmd/gohelp -q testflag -json
```

The CLI prints matching snippets and help paths. Use `-json` for a JSON array of matches and `-go` to select the executable.

## Development

```text
gofmt -w .
GOCACHE=/private/tmp/gohelp-gocache go test ./...
go vet ./...
```
