// Package fstree provides a lazy, per-directory filesystem scanner for the
// Explorer sidebar. It is a pure, Bubble-Tea-free package: callers wrap
// Scan in a tea.Cmd and dispatch its result as a message.
package fstree

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultIgnore is the default set of directory/file names skipped when
// scanning — overridable via flag/config.
var DefaultIgnore = []string{".git", "node_modules", "vendor", "target"}

// FileNode is one entry in the Explorer's directory tree.
type FileNode struct {
	Name     string
	Path     string // absolute path
	IsDir    bool
	IsMD     bool // .md / .markdown, cached at scan time
	Expanded bool // dirs only
	Children []*FileNode
	Depth    int // for indent rendering, set at scan/flatten time
}

// NewRoot creates the (unscanned) root node for path. Callers should follow
// up with ScanDir(path, ignore, nil) to populate its children.
func NewRoot(path string) *FileNode {
	return &FileNode{
		Name:  filepath.Base(path),
		Path:  path,
		IsDir: true,
	}
}

// ScanDir lists the immediate children of dir, skipping names in ignore and
// refusing to descend into a path already present in visited (symlink
// cycle guard — pass a fresh map per top-level scan call and let callers
// thread it through repeated ScanDir calls for the same walk if needed).
// Results are sorted directories-first, then alphabetically.
func ScanDir(dir string, ignore []string, visited map[string]bool) ([]*FileNode, error) {
	real := dir
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		real = resolved
	}
	if visited != nil {
		if visited[real] {
			return nil, nil
		}
		visited[real] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	ignoreSet := make(map[string]bool, len(ignore))
	for _, name := range ignore {
		ignoreSet[name] = true
	}

	nodes := make([]*FileNode, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if ignoreSet[name] {
			continue
		}
		isDir := e.IsDir()
		if !isDir && e.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		ext := strings.ToLower(filepath.Ext(name))
		nodes = append(nodes, &FileNode{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: isDir,
			IsMD:  !isDir && (ext == ".md" || ext == ".markdown"),
		})
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}
