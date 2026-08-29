package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/MouXiaoJun/gohelp"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var query string
	var command string
	var cache string
	var jsonOutput bool
	var refresh bool
	flags := flag.NewFlagSet("gohelp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&query, "q", "", "keyword or flag to search")
	flags.StringVar(&command, "go", "go", "go executable to query")
	flags.StringVar(&cache, "cache", "", "index cache file")
	flags.BoolVar(&jsonOutput, "json", false, "write search results as JSON")
	flags.BoolVar(&refresh, "refresh", false, "rebuild the index cache")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if query == "" {
		query = strings.Join(flags.Args(), " ")
	}
	if strings.TrimSpace(query) == "" {
		flags.Usage()
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	index, err := loadOrBuild(ctx, command, cache, refresh)
	if err != nil {
		fmt.Fprintf(stderr, "gohelp: %v\n", err)
		return 1
	}
	matches := index.Search(query)
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(matches); err != nil {
			fmt.Fprintf(stderr, "gohelp: write JSON: %v\n", err)
			return 1
		}
		return 0
	}
	if len(matches) == 0 {
		fmt.Fprintf(stdout, "No help matches for %q.\n", query)
		return 0
	}
	for _, match := range matches {
		fmt.Fprintf(stdout, "%s\n  %s\n", match.Topic, match.HelpPath)
		for _, snippet := range match.Snippets {
			fmt.Fprintf(stdout, "    %s\n", snippet)
		}
	}
	return 0
}

func loadOrBuild(ctx context.Context, command, cache string, refresh bool) (*gohelp.Index, error) {
	if cache == "" {
		return gohelp.New(ctx, gohelp.Options{GoCommand: command})
	}
	if !refresh {
		if index, err := gohelp.Load(cache); err == nil && index.GoCommand == command {
			version, err := gohelp.Version(ctx, gohelp.Options{GoCommand: command})
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(index.GoVersion) == version {
				return index, nil
			}
		}
	}
	index, err := gohelp.New(ctx, gohelp.Options{GoCommand: command})
	if err != nil {
		return nil, err
	}
	if err := index.Save(cache); err != nil {
		return nil, err
	}
	return index, nil
}
