package spec

// The writer: Session back to YAML that parses to the same Session (see the round-trip
// tests). Uses yaml.Node for order and !!int pane keys.

import (
	"fmt"

	"github.com/brohd11/tmux_s/internal/pathx"
	"gopkg.in/yaml.v3"
)

// Marshal renders one session in the single-session form, writing each directory at the
// highest level that covers it.
func Marshal(s Session) ([]byte, error) {
	node, err := Node(s)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(node)
}

// Node builds the document Marshal encodes. It is separate so a caller with its own
// encoder settings, or one writing through configdir.SaveAtomic, can hand off the tree.
func Node(s Session) (*yaml.Node, error) {
	if s.Name == "" {
		return nil, fmt.Errorf("session has an empty name")
	}
	if len(s.Windows) == 0 {
		return nil, fmt.Errorf("session %q: no windows", s.Name)
	}

	root := mapping()
	// Always written, so the filename doesn't rename the session.
	put(root, "session", str(s.Name))
	if s.Dir != "" {
		put(root, "dir", str(pathx.Contract(s.Dir)))
	}

	windows := mapping()
	seen := map[string]bool{}
	for _, w := range s.Windows {
		if w.Name == "" {
			return nil, fmt.Errorf("session %q: window has an empty name", s.Name)
		}
		if seen[w.Name] {
			return nil, fmt.Errorf("session %q: duplicate window name %q", s.Name, w.Name)
		}
		seen[w.Name] = true
		node, err := windowNode(w, s.Dir)
		if err != nil {
			return nil, fmt.Errorf("session %q: %w", s.Name, err)
		}
		put(windows, w.Name, node)
	}
	put(root, "windows", windows)

	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}, nil
}

// windowNode renders one window. inherit is the directory the window falls back to, so a
// window that adds nothing to it writes no `dir` of its own.
func windowNode(w Window, inherit string) (*yaml.Node, error) {
	if len(w.Panes) == 0 {
		return nil, fmt.Errorf("window %q: no panes", w.Name)
	}

	// A directory shared by every pane goes on the window.
	dir := w.Dir
	if dir == "" {
		if common, ok := commonDir(w.Panes); ok {
			dir = common
		}
	}
	paneInherit := inherit
	if dir != "" {
		paneInherit = dir
	}

	node := mapping()
	if dir != "" && dir != inherit {
		put(node, "dir", str(pathx.Contract(dir)))
	}
	// A layout only ever reaches tmux for a window that was split, so writing one for a
	// single pane would record a string nothing reads.
	if w.Layout != "" && len(w.Panes) > 1 {
		put(node, "layout", str(w.Layout))
	}
	if w.Focus {
		put(node, "focus", boolean(true))
	}

	// The single-pane form works only when pane and window share a directory.
	if len(w.Panes) == 1 && w.Panes[0].Dir == paneInherit {
		paneFields(node, w.Panes[0], paneInherit, false)
		return node, nil
	}

	for i, p := range w.Panes {
		pane := mapping()
		paneFields(pane, p, paneInherit, true)
		putInt(node, i, pane)
	}
	return node, nil
}

// paneFields adds a pane's keys. withDir and focus are skipped for the single-pane form.
func paneFields(node *yaml.Node, p Pane, inherit string, withDir bool) {
	if withDir && p.Dir != "" && p.Dir != inherit {
		put(node, "dir", str(pathx.Contract(p.Dir)))
	}
	if len(p.Keys) == 1 {
		put(node, "keys", str(p.Keys[0]))
	} else if len(p.Keys) > 1 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, k := range p.Keys {
			seq.Content = append(seq.Content, str(k))
		}
		put(node, "keys", seq)
	}
	// Enter defaults to true in the reader, so only the false case has to be written —
	// and only where there is a line for it to suppress.
	if len(p.Keys) > 0 && !p.Enter {
		put(node, "enter", boolean(false))
	}
	if withDir && p.Focus {
		put(node, "focus", boolean(true))
	}
}

// commonDir reports the directory every pane shares, if they all share one.
func commonDir(panes []Pane) (string, bool) {
	first := panes[0].Dir
	if first == "" {
		return "", false
	}
	for _, p := range panes[1:] {
		if p.Dir != first {
			return "", false
		}
	}
	return first, true
}

// mapping is a block mapping that renders as `{}` when empty. A bare `name:` would read back
// as a null scalar, i.e. a pane running an empty command.
func mapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
}

// put appends a key/value pair, restoring block style now that the mapping is not empty.
func put(m *yaml.Node, key string, val *yaml.Node) {
	m.Style = 0
	m.Content = append(m.Content, str(key), val)
}

// putInt appends a pane, whose key must be an integer: the !!int tag is what tells the
// reader a pane from one of the window's own fields.
func putInt(m *yaml.Node, key int, val *yaml.Node) {
	m.Style = 0
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(key)}, val)
}

// str is a string scalar tagged !!str, so names like `0`, `true` or `null` round-trip.
func str(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func boolean(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(v)}
}
