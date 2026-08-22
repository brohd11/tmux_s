// Package spec parses the YAML session files into the plain Session/Window/Pane
// structs the planner consumes.
//
// Everything here goes through yaml.Node rather than a struct or map decode, for two
// reasons that are both fatal to the simpler approach:
//
//   - Go maps are unordered, and window/pane order is the entire point of the file.
//     Only a Node walk preserves the order the user wrote.
//   - A pane key is written as a bare integer (`0:`), which YAML tags !!int. Decoding
//     that into map[string]any fails outright, and the int-vs-string test on the key is
//     precisely what tells a window's panes apart from its own fields.
//
// The Node handling is confined to this package; everything downstream sees structs.
package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Session is one tmux session: a name, a default working directory, and its windows in
// the order they should be created.
type Session struct {
	Name    string
	Dir     string
	Windows []Window
}

// Window is one tmux window. Panes is never empty — a window written without integer
// keys still yields the single pane tmux creates with the window itself.
type Window struct {
	Name   string
	Dir    string
	Layout string
	Focus  bool
	Panes  []Pane
}

// Pane is one tmux pane and the keystrokes sent to it.
type Pane struct {
	Dir   string
	Keys  []string
	Enter bool // send C-m after each line; true unless the file says enter: false
	Focus bool
}

// ParseFile reads one session file and returns every session it defines.
//
// A top-level mapping carrying any of a session's own keys (`windows`, `session`,
// `dir`) is a single session, named by its `session:` key or, failing that, the file's
// basename. Any other mapping is read as a map of session name to spec, which is how
// several sessions share one file.
func ParseFile(path string) ([]Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// An empty file decodes to a zero Node with no content — no sessions, no error.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping at the top level, got %s", path, kindName(root.Kind))
	}

	if isSingleSession(root) {
		s, err := parseSession(defaultName(path), root)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return []Session{s}, nil
	}

	var out []Session
	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		body := root.Content[i+1]
		if body.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s: session %q: expected a mapping, got %s", path, name, kindName(body.Kind))
		}
		s, err := parseSession(name, body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// defaultName is the session name a single-session file gets when it declares no
// `session:` key: the basename with its extension stripped.
func defaultName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// parseSession reads one session mapping. name is the fallback the caller derived; an
// explicit `session:` key overrides it.
func parseSession(name string, node *yaml.Node) (Session, error) {
	s := Session{Name: name}
	var windows *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "session":
			s.Name = val.Value
		case "dir":
			s.Dir = expandPath(val.Value)
		case "windows":
			windows = val
		default:
			return s, fmt.Errorf("session %q: unknown key %q", s.Name, key.Value)
		}
	}
	if s.Name == "" {
		return s, fmt.Errorf("session has an empty name")
	}
	if windows == nil {
		return s, fmt.Errorf("session %q: no windows", s.Name)
	}
	if windows.Kind != yaml.MappingNode {
		return s, fmt.Errorf("session %q: windows must be a mapping, got %s", s.Name, kindName(windows.Kind))
	}
	for i := 0; i+1 < len(windows.Content); i += 2 {
		w, err := parseWindow(windows.Content[i].Value, windows.Content[i+1], s.Dir)
		if err != nil {
			return s, fmt.Errorf("session %q: %w", s.Name, err)
		}
		s.Windows = append(s.Windows, w)
	}
	if len(s.Windows) == 0 {
		return s, fmt.Errorf("session %q: no windows", s.Name)
	}
	return s, nil
}

// parseWindow reads one window. inherit is the directory a pane falls back to when
// neither it nor the window names one.
//
// The value is either a bare string — shorthand for a single pane running that line —
// or a mapping. In a mapping, integer keys are panes and string keys are the window's
// own fields; a mapping mixing the two has no sensible reading and is rejected.
func parseWindow(name string, node *yaml.Node, inherit string) (Window, error) {
	w := Window{Name: name}

	if node.Kind == yaml.ScalarNode {
		p, err := scalarPane(node, inherit)
		if err != nil {
			return w, fmt.Errorf("window %q: %w", name, err)
		}
		w.Panes = []Pane{p}
		return w, nil
	}
	if node.Kind != yaml.MappingNode {
		return w, fmt.Errorf("window %q: expected a mapping or a string, got %s", name, kindName(node.Kind))
	}

	type indexed struct {
		n    int
		node *yaml.Node
	}
	var panes []indexed
	var fields []*yaml.Node // key nodes with string keys, paired with Content[i+1]
	seen := map[int]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		if key.Tag == "!!int" {
			n, err := strconv.Atoi(key.Value)
			if err != nil {
				return w, fmt.Errorf("window %q: bad pane index %q: %w", name, key.Value, err)
			}
			if seen[n] {
				return w, fmt.Errorf("window %q: duplicate pane index %d", name, n)
			}
			seen[n] = true
			panes = append(panes, indexed{n, val})
			continue
		}
		fields = append(fields, key, val)
	}

	windowKeys := map[string]bool{"dir": true, "layout": true, "focus": true}
	for i := 0; i+1 < len(fields); i += 2 {
		key, val := fields[i], fields[i+1]
		if len(panes) > 0 && !windowKeys[key.Value] {
			// `keys`/`enter` belong to a pane. Alongside integer keys there is no pane
			// for them to attach to, and silently dropping them would lose commands.
			return w, fmt.Errorf("window %q: %q cannot be combined with numbered panes — move it into a pane", name, key.Value)
		}
		switch key.Value {
		case "dir":
			w.Dir = expandPath(val.Value)
		case "layout":
			w.Layout = val.Value
		case "focus":
			b, err := parseBool(val)
			if err != nil {
				return w, fmt.Errorf("window %q: focus: %w", name, err)
			}
			w.Focus = b
		}
	}

	paneInherit := inherit
	if w.Dir != "" {
		paneInherit = w.Dir
	}

	if len(panes) == 0 {
		// No integer keys: the mapping describes the window's single pane.
		p, err := parsePane(node, paneInherit, windowKeys)
		if err != nil {
			return w, fmt.Errorf("window %q: %w", name, err)
		}
		w.Panes = []Pane{p}
		return w, nil
	}

	// Panes run in index order, not document order: writing 1 before 0 still means
	// pane 0 comes first, and the indices need not be a contiguous run.
	sort.Slice(panes, func(i, j int) bool { return panes[i].n < panes[j].n })
	for _, p := range panes {
		pane, err := parsePaneValue(p.node, paneInherit)
		if err != nil {
			return w, fmt.Errorf("window %q: pane %d: %w", name, p.n, err)
		}
		w.Panes = append(w.Panes, pane)
	}
	return w, nil
}

// parsePaneValue reads a pane written either as a bare string or as a mapping.
func parsePaneValue(node *yaml.Node, inherit string) (Pane, error) {
	if node.Kind == yaml.ScalarNode {
		return scalarPane(node, inherit)
	}
	if node.Kind != yaml.MappingNode {
		return Pane{}, fmt.Errorf("expected a mapping or a string, got %s", kindName(node.Kind))
	}
	return parsePane(node, inherit, nil)
}

// scalarPane builds the pane a bare string denotes: `term: cd ~/main/go`.
func scalarPane(node *yaml.Node, inherit string) (Pane, error) {
	return Pane{Dir: inherit, Keys: []string{node.Value}, Enter: true}, nil
}

// parsePane reads a pane mapping. skip names the keys that belong to the enclosing
// window rather than the pane — non-nil only for the single-pane form, where window and
// pane share one mapping.
func parsePane(node *yaml.Node, inherit string, skip map[string]bool) (Pane, error) {
	p := Pane{Dir: inherit, Enter: true}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		if skip[key.Value] {
			continue
		}
		switch key.Value {
		case "dir":
			p.Dir = expandPath(val.Value)
		case "keys":
			keys, err := parseKeys(val)
			if err != nil {
				return p, err
			}
			p.Keys = keys
		case "enter":
			b, err := parseBool(val)
			if err != nil {
				return p, fmt.Errorf("enter: %w", err)
			}
			p.Enter = b
		case "focus":
			b, err := parseBool(val)
			if err != nil {
				return p, fmt.Errorf("focus: %w", err)
			}
			p.Focus = b
		default:
			return p, fmt.Errorf("unknown key %q", key.Value)
		}
	}
	return p, nil
}

// parseKeys accepts either one line or a list of them; a list sends one send-keys per
// entry, which is how a pane runs several commands in sequence.
func parseKeys(node *yaml.Node) ([]string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{node.Value}, nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, c := range node.Content {
			if c.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("keys: list entries must be strings, got %s", kindName(c.Kind))
			}
			out = append(out, c.Value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("keys: expected a string or a list, got %s", kindName(node.Kind))
	}
}

func parseBool(node *yaml.Node) (bool, error) {
	var b bool
	if err := node.Decode(&b); err != nil {
		return false, fmt.Errorf("expected true or false, got %q", node.Value)
	}
	return b, nil
}

// isSingleSession decides which of the two file shapes this is. Any of the session's
// own keys at the top level marks the single-session form; without them every top-level
// key is a session name.
//
// `session` and `dir` count, not just `windows`: a file with `session:` and a typo for
// `windows:` would otherwise be read as a session literally named "session", and the
// error would point at the wrong thing entirely.
func isSingleSession(node *yaml.Node) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "windows", "session", "dir":
			return true
		}
	}
	return false
}

// expandPath resolves ~ and $VAR in a directory so a session file can be shared across
// machines whose home directories differ. tmux's -c takes a literal path and does no
// expansion of its own, and nothing here goes through a shell that would.
func expandPath(p string) string {
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
		}
	}
	return p
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "a document"
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a mapping"
	case yaml.ScalarNode:
		return "a string"
	case yaml.AliasNode:
		return "an alias"
	default:
		return "nothing"
	}
}
