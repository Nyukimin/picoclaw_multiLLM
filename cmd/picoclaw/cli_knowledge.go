package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	knowledgeapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/knowledge"
)

func cmdKnowledge() {
	configPath := getConfigPath()
	store, err := loadSourceRegistryStore(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize knowledge store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	code := runKnowledgeCommand(os.Args[2:], store, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

type knowledgeCLIStore interface {
	knowledgeapp.StagingStore
}

func runKnowledgeCommand(args []string, store knowledgeCLIStore, out io.Writer, errOut io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch subcmd {
	case "import-core-jsonl":
		jsonOut := hasFlag(args[1:], "--json")
		var inputPath string
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "--") {
				continue
			}
			inputPath = arg
			break
		}
		if strings.TrimSpace(inputPath) == "" {
			fmt.Fprintln(errOut, "usage: picoclaw knowledge import-core-jsonl <path> [--json]")
			return 1
		}
		f, err := os.Open(inputPath)
		if err != nil {
			fmt.Fprintf(errOut, "failed to open knowledge jsonl: %v\n", err)
			return 1
		}
		defer f.Close()
		result, err := knowledgeapp.ImportKnowledgeCoreJSONL(context.Background(), store, f, knowledgeapp.ImportOptions{})
		if err != nil {
			fmt.Fprintf(errOut, "failed to import knowledge jsonl: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"imported": result.Imported}, false)
			return 0
		}
		fmt.Fprintf(out, "imported knowledge core records: %d\n", result.Imported)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown knowledge subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw knowledge import-core-jsonl <path>")
		return 1
	}
}
