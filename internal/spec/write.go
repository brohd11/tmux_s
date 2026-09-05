package spec

// The writer half of the package: a Session back to the YAML a reader would parse into
// the same Session. It is here rather than beside its caller so the two halves of the
// format sit together — every rule ParseFile enforces has its mirror a few hundred lines
// up, and a change to one that misses the other shows as a failing round-trip test.
//
// Like the reader, it works in yaml.Node rather than marshalling a Go map, for the same
// two reasons: a map would scramble window and pane order, and a pane's key has to carry
// the !!int tag that tells panes apart from the window's own fields.

import (
	"fmt"

	"github.com/brohd11/tmux_s/internal/pathx"
	"gopkg.in/yaml.v3"
)

// Marshal renders one session as a session file, in the single-session form.
//
// Directories are written at the highest level that covers them — a directory every pane
// in a window shares is written once on the window — because the reader resolves
// inheritance while parsing, leaving every Pane.Dir populated. Dumping those verbatim
// would put a `dir` on every pane of every window and bury the parts worth reading.
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
	// `session` is always written, never left to the filename. A file saved under a name
	// of its own — `tmux_s save go-dev ~/my-session.yaml` — would otherwise come back as a
	// session called my-session, silently renamed by where it was put.
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

	// A directory shared by every pane belongs on the window; anything else stays on the
	// panes that differ. Written on the window it also becomes their inherit, so the
	// panes below fall silent.
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

	// The single-pane form folds the pane into the window's own mapping, which only
	// works while the two agree on a directory — there is one `dir` key between them, and
	// the reader hands it to both. A pane that overrides its window's directory has to be
	// written out as a numbered pane instead, even though it is alone.
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

// paneFields adds a pane's keys to node. withDir is false for the single-pane form, where
// the directory was already written as the window's and repeating it would be read back
// as the window's anyway. focus is only meaningful where there is more than one pane to
// choose between, which is the same condition.
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

// mapping is a block mapping that renders as `{}` while it is empty.
//
// The empty case is the reason the style is set at all. A window with nothing to say —
// no directory of its own, no layout, no focus, no keys — must still be a mapping: left
// as a bare `name:` it is a null scalar, and the reader turns a scalar window into a pane
// running that string, so every rebuild would send an empty line to the pane. `{}` reads
// back as the single default pane it is. yaml.v3 drops the flow style once the mapping
// has content, so the non-empty case is unaffected.
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

// str is a string scalar. The explicit !!str tag makes yaml.v3 quote a value that would
// otherwise read back as something else, so a window named `0`, `true` or `null` survives
// the round trip as its name.
func str(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func boolean(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(v)}
}
