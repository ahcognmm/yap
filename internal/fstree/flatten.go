package fstree

// FlatRow is what the tree view actually iterates for rendering and cursor
// movement — flattening avoids re-walking the tree on every keypress.
type FlatRow struct {
	Node  *FileNode
	Depth int
}

// Flatten walks root's children (root itself is not included as a row) into
// a depth-first slice, descending into a directory only when Expanded.
func Flatten(root *FileNode) []FlatRow {
	var rows []FlatRow
	var walk func(n *FileNode, depth int)
	walk = func(n *FileNode, depth int) {
		for _, child := range n.Children {
			child.Depth = depth
			rows = append(rows, FlatRow{Node: child, Depth: depth})
			if child.IsDir && child.Expanded {
				walk(child, depth+1)
			}
		}
	}
	walk(root, 0)
	return rows
}
