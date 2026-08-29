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

## CLI

```text
go run ./cmd/gohelp -q=-modfile
go run ./cmd/gohelp -q testflag -json
go run ./cmd/gohelp -cache gohelp-cache.json -q=-modfile
```

The CLI prints matching snippets and help paths. Use `-json` for a JSON array of matches, `-go` to select the executable, and `-cache` to load or refresh an index cache. Without `-cache`, the CLI keeps rebuilding the index as before.

## Development

```text
gofmt -w .
GOWORK=off go test ./...
go vet ./...
```
