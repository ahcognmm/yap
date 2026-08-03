// Command yap is a Jupyter-style TUI for Markdown: an Explorer file-tree
// sidebar and a Document notebook pane with in-place, streamed shell
// execution. See DESIGN.md for the full spec.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v3"

	"yap/internal/fstree"
	"yap/internal/theme"
	"yap/internal/ui"
)

func main() {
	cmd := &cli.Command{
		Name:      "yap",
		Usage:     "a Jupyter-style TUI for Markdown",
		ArgsUsage: "[path]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "theme",
				Value: "default",
				Usage: "color theme (default, catppuccin)",
			},
			&cli.StringSliceFlag{
				Name:  "ignore",
				Usage: "additional names to ignore when scanning (comma-separated)",
			},
		},
		Action: run,
	}

	// A plain (non-ExitCoder) error is returned untouched by Run rather than
	// printed by the library, so this stays the single place errors surface.
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "yap:", err)
		os.Exit(1)
	}
}

func run(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() > 1 {
		return fmt.Errorf("accepts at most 1 path argument, got %d", cmd.NArg())
	}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("yap requires an interactive terminal")
	}

	path := "."
	if cmd.NArg() == 1 {
		path = cmd.Args().First()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}

	rootDir := abs
	openFile := ""
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(abs))
		if ext != ".md" && ext != ".markdown" {
			return fmt.Errorf("%s is not a directory or a Markdown file", abs)
		}
		rootDir = filepath.Dir(abs)
		openFile = abs
	}

	th := theme.ByName(cmd.String("theme"))
	ignoreList := fstree.DefaultIgnore
	if ignore := cmd.StringSlice("ignore"); len(ignore) > 0 {
		ignoreList = append(append([]string{}, fstree.DefaultIgnore...), ignore...)
	}

	m := ui.New(rootDir, openFile, th, ignoreList)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}
