package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parse writes body to a temp file named name and parses it, so every case exercises
// the real filename-derived session naming rather than a hand-built node.
func parse(t *testing.T, name, body string) ([]Session, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return ParseFile(path)
}

func mustParse(t *testing.T, name, body string) []Session {
	t.Helper()
	got, err := parse(t, name, body)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return got
}

// The order windows are written in is the order they must be created in, and a map
// decode would lose it — this is the reason the parser walks yaml.Node at all.
func TestWindowOrderFollowsTheFile(t *testing.T) {
	s := mustParse(t, "go-dev.yaml", `
windows:
  agent: claude
  repo: repoview
  term: cd ~/main/go
`)[0]
	if s.Name != "go-dev" {
		t.Errorf("name = %q, want go-dev (from the filename)", s.Name)
	}
	var got []string
	for _, w := range s.Windows {
		got = append(got, w.Name)
	}
	if strings.Join(got, ",") != "agent,repo,term" {
		t.Errorf("windows = %v, want [agent repo term]", got)
	}
}

// A bare string window is shorthand for a single pane running that line.
func TestStringShorthandIsOnePane(t *testing.T) {
	w := mustParse(t, "s.yaml", "windows:\n  term: cd ~/main/go\n")[0].Windows[0]
	if len(w.Panes) != 1 {
		t.Fatalf("panes = %d, want 1", len(w.Panes))
	}
	if got := w.Panes[0].Keys; len(got) != 1 || got[0] != "cd ~/main/go" {
		t.Errorf("keys = %v, want [cd ~/main/go]", got)
	}
	if !w.Panes[0].Enter {
		t.Error("enter = false, want true by default")
	}
}

// Integer keys mean panes; the absence of any means the mapping describes the window's
// one pane. Getting this discrimination wrong is the whole schema's failure mode.
func TestIntKeysBecomePanes(t *testing.T) {
	s := mustParse(t, "s.yaml", `
windows:
  single:
    keys: gote notes
    enter: false
  split:
    layout: even-horizontal
    0:
      keys: ssh -L 1:localhost:1 brohd
    1:
      keys: [clear, tail -f out.log]
`)[0]

	single := s.Windows[0]
	if len(single.Panes) != 1 {
		t.Fatalf("single: panes = %d, want 1", len(single.Panes))
	}
	if single.Panes[0].Enter {
		t.Error("single: enter = true, want false")
	}

	split := s.Windows[1]
	if len(split.Panes) != 2 {
		t.Fatalf("split: panes = %d, want 2", len(split.Panes))
	}
	if split.Layout != "even-horizontal" {
		t.Errorf("split: layout = %q", split.Layout)
	}
	if got := split.Panes[1].Keys; len(got) != 2 || got[1] != "tail -f out.log" {
		t.Errorf("split: pane 1 keys = %v, want two lines", got)
	}
}

// Panes run in index order, not document order: a file listing 1 before 0 still means
// pane 0 comes first.
func TestPanesSortByIndexNotDocumentOrder(t *testing.T) {
	w := mustParse(t, "s.yaml", `
windows:
  w:
    2: third
    0: first
    1: second
`)[0].Windows[0]
	var got []string
	for _, p := range w.Panes {
		got = append(got, p.Keys[0])
	}
	if strings.Join(got, ",") != "first,second,third" {
		t.Errorf("panes = %v, want [first second third]", got)
	}
}

// dir falls pane → window → session, so `dir: ~/main/go` at the top replaces a `cd`
// prefix on every line below it.
func TestDirInherits(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	s := mustParse(t, "s.yaml", `
dir: ~/main/go
windows:
  a: claude
  b:
    dir: /tmp
    0: one
    1:
      dir: /var
      keys: two
`)[0]
	if s.Dir != filepath.Join(home, "main/go") {
		t.Errorf("session dir = %q, want the expanded ~/main/go", s.Dir)
	}
	if got := s.Windows[0].Panes[0].Dir; got != s.Dir {
		t.Errorf("window a pane dir = %q, want the session's %q", got, s.Dir)
	}
	if got := s.Windows[1].Panes[0].Dir; got != "/tmp" {
		t.Errorf("window b pane 0 dir = %q, want the window's /tmp", got)
	}
	if got := s.Windows[1].Panes[1].Dir; got != "/var" {
		t.Errorf("window b pane 1 dir = %q, want its own /var", got)
	}
}

// A `session:` key overrides the filename, and a top level with no `windows:` is a map
// of several sessions.
func TestNamingForms(t *testing.T) {
	s := mustParse(t, "anything.yaml", "session: renamed\nwindows:\n  w: x\n")
	if len(s) != 1 || s[0].Name != "renamed" {
		t.Fatalf("explicit session key: got %v", s)
	}

	multi := mustParse(t, "all.yaml", `
one:
  windows:
    w: a
two:
  windows:
    w: b
`)
	if len(multi) != 2 || multi[0].Name != "one" || multi[1].Name != "two" {
		t.Fatalf("multi-session file: got %v", multi)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"pane keys mixed with window keys", "windows:\n  w:\n    keys: a\n    0: b\n", "numbered panes"},
		{"duplicate pane index", "windows:\n  w:\n    0: a\n    0: b\n", "duplicate pane index 0"},
		{"unknown session key", "nope: 1\nwindows:\n  w: a\n", "unknown key"},
		{"unknown pane key", "windows:\n  w:\n    kesy: a\n", "unknown key"},
		{"no windows", "session: x\n", "no windows"},
		{"windows not a mapping", "windows: [a, b]\n", "must be a mapping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(t, "s.yaml", tc.body)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// An empty file is not a broken file; it simply defines nothing.
func TestEmptyFile(t *testing.T) {
	got, err := parse(t, "s.yaml", "")
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}
