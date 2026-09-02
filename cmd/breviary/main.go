// Command breviary — runbook automation engine
// usage:
//
//	breviary serve             — HTTP daemon
//	breviary run <file>        — run runbook
//	breviary resume <run-id>   — resume failed run
//	breviary validate <file>   — validate runbook YAML
//	breviary logs <run-id>     — audit trail
//	breviary approve <run-id> <step-id>  — CLI approval
//	breviary reject <run-id> <step-id>   — CLI reject
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/breviary/breviary/internal/engine"
	"github.com/breviary/breviary/internal/git"
	"github.com/breviary/breviary/internal/server"
	"github.com/breviary/breviary/internal/spec"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "run":
		err = cmdRun(args)
	case "resume":
		err = cmdResume(args)
	case "validate":
		err = cmdValidate(args)
	case "logs":
		err = cmdLogs(args)
	case "git":
		err = cmdGit(args)
	case "approve":
		err = cmdApprove(args)
	case "reject":
		err = cmdReject(args)
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Breviary — runbook automation engine

Usage:
  breviary serve             — HTTP daemon (webhook + approval API)
  breviary run <file>        — run runbook from YAML
  breviary resume <run-id>   — resume failed/approval-pending run
  breviary validate <file>   — validate runbook YAML
  breviary logs <run-id>     — show audit trail
  breviary git <url> [ref]   — clone/pull runbook repository
  breviary approve <run-id> <step-id>  — approve pending step
  breviary reject <run-id> <step-id>   — reject pending step
`)
}

func getEngine(dbPath string) *engine.Engine {
	if dbPath == "" {
		dbPath = "breviary.db"
	}
	eng, err := engine.New(engine.WithJournalPath(dbPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: engine: %v\n", err)
		os.Exit(1)
	}
	return eng
}

func cmdServe(args []string) error {
	addr := ":8080"
	if len(args) > 0 {
		addr = args[0]
	}

	eng := getEngine("breviary.db")
	defer eng.Close()

	srv := server.New(eng)

	// Load all .yaml files from the runbooks/ directory
	entries, err := os.ReadDir("runbooks")
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
				book, err := spec.ValidateYAML("runbooks/" + e.Name())
				if err == nil {
					srv.RegisterRunbook(book)
					fmt.Fprintf(os.Stderr, "  loaded runbook: %s\n", book.Metadata.Name)
				}
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Fprintf(os.Stderr, "Breviary serve on %s\n", addr)
		if err := srv.Listen(addr); err != nil {
			fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
		}
	}()

	<-ctx.Done()
	fmt.Fprintln(os.Stderr, "\nShutting down...")
	srv.Shutdown(context.Background())
	return nil
}

func cmdRun(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: breviary run <file> [--db <path>]")
	}
	path := args[0]

	eng := getEngine("")
	defer eng.Close()

	book, err := spec.ValidateYAML(path)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	n := len(book.Spec.Steps)
	fmt.Fprintf(os.Stderr, "▶ Running runbook '%s' (%d steps)\n", book.Metadata.Name, n)

	ctx := context.Background()
	if err := eng.Run(ctx, book, ""); err != nil {
		if err.Error() == "approval pending" {
			fmt.Fprintln(os.Stderr, "⏳ Step requires approval (breviary approve <run-id> <step-id>)")
			return nil
		}
		return fmt.Errorf("%w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ Done")
	return nil
}

func cmdResume(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: breviary resume <run-id>")
	}
	runID := args[0]

	eng := getEngine("")
	defer eng.Close()

	ctx := context.Background()
	if err := eng.Resume(ctx, runID); err != nil {
		if err.Error() == "approval pending" {
			fmt.Fprintln(os.Stderr, "⏳ Step requires approval")
			return nil
		}
		return fmt.Errorf("%w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ Run completed")
	return nil
}

func cmdValidate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: breviary validate <file>")
	}
	for _, path := range args {
		book, err := spec.ValidateYAML(path)
		if err != nil {
			return fmt.Errorf("%s — %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "✓ %s — valid runbook (%d steps)\n", book.Metadata.Name, len(book.Spec.Steps))
	}
	return nil
}

func cmdLogs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: breviary logs <run-id>")
	}
	runID := args[0]

	eng := getEngine("")
	defer eng.Close()

	events, err := eng.Events(runID)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	fmt.Fprintf(os.Stderr, "Run: %s\n", runID)
	for _, ev := range events {
		fmt.Fprintf(os.Stderr, "  %s/%s → %s\n", ev.RunID, ev.StepID, ev.Kind)
	}
	return nil
}

func cmdGit(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: breviary git <clone-url> [ref]")
	}
	repoURL := args[0]
	ref := "main"
	if len(args) > 1 {
		ref = args[1]
	}

	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "breviary", "runbooks")
	dir, err := git.Sync(cacheDir, repoURL, ref)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	files, _ := git.ListRunbooks(dir)
	fmt.Fprintf(os.Stderr, "✓ Repository %s synchronized\n", repoURL)
	for _, f := range files {
		fmt.Fprintf(os.Stderr, "  • %s\n", f)
	}
	return nil
}

func cmdApprove(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("Usage: breviary approve <run-id> <step-id>")
	}
	runID, stepID := args[0], args[1]

	eng := getEngine("")
	defer eng.Close()

	if err := eng.ApproveStep(runID, stepID); err != nil {
		return fmt.Errorf("%w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Step %s/%s approved\n", runID, stepID)
	return nil
}

func cmdReject(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("Usage: breviary reject <run-id> <step-id>")
	}
	runID, stepID := args[0], args[1]

	eng := getEngine("")
	defer eng.Close()

	if err := eng.RejectStep(runID, stepID); err != nil {
		return fmt.Errorf("%w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Step %s/%s rejected\n", runID, stepID)
	return nil
}
