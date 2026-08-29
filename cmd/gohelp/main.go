package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/MouXiaoJun/gohelp"
)

func main() {
	var query string
	var command string
	var jsonOutput bool
	flag.StringVar(&query, "q", "", "keyword or flag to search")
	flag.StringVar(&command, "go", "go", "go executable to query")
	flag.BoolVar(&jsonOutput, "json", false, "write search results as JSON")
	flag.Parse()
	if query == "" {
		query = strings.Join(flag.Args(), " ")
	}
	if strings.TrimSpace(query) == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	index, err := gohelp.New(ctx, gohelp.Options{GoCommand: command})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gohelp: %v\n", err)
		os.Exit(1)
	}
	matches := index.Search(query)
	if jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(matches); err != nil {
			fmt.Fprintf(os.Stderr, "gohelp: write JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(matches) == 0 {
		fmt.Printf("No help matches for %q.\n", query)
		return
	}
	for _, match := range matches {
		fmt.Printf("%s\n  %s\n", match.Topic, match.HelpPath)
		for _, snippet := range match.Snippets {
			fmt.Printf("    %s\n", snippet)
		}
	}
}
