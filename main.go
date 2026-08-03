// Command runmd is a Jupyter-style TUI for Markdown: an Explorer file-tree
// sidebar and a Document notebook pane with in-place, streamed shell
// execution. See DESIGN.md for the full spec.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"runmd/internal/fstree"
	"runmd/internal/theme"
	"runmd/internal/ui"
)

func main() {
	var (
		themeName string
		ignore    []string
	)

	root := &cobra.Command{
		Use:   "runmd [path]",
		Short: "runmd — a Jupyter-style TUI for Markdown",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isatty.IsTerminal(os.Stdout.Fd()) {
				return fmt.Errorf("runmd requires an interactive terminal")
			}

			path := "."
			if len(args) == 1 {
				path = args[0]
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

			th := theme.ByName(themeName)
			ignoreList := fstree.DefaultIgnore
			if len(ignore) > 0 {
				ignoreList = append(append([]string{}, fstree.DefaultIgnore...), ignore...)
			}

			m := ui.New(rootDir, openFile, th, ignoreList)
			p := tea.NewProgram(m)
			_, err = p.Run()
			return err
		},
	}

	root.Flags().StringVar(&themeName, "theme", "default", "color theme (default, catppuccin)")
	root.Flags().StringSliceVar(&ignore, "ignore", nil, "additional names to ignore when scanning (comma-separated)")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "runmd:", err)
		os.Exit(1)
	}
}
