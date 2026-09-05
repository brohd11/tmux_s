package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func marshal(t *testing.T, s Session) string {
	t.Helper()
	out, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(out)
}

// The output is asserted as text, the way plan_test asserts the printed commands: the
// file is what a person opens and edits, so its exact shape is the thing worth pinning.
func TestMarshal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir := filepath.Join(home, "main", "go")

	tests := []struct {
		name string
		in   Session
		want string
	}{
		{
			name: "a captured session, directories collapsed onto the session",
			in: Session{
				Name: "go-dev",
				Dir:  dir,
				Windows: []Window{
					{Name: "agent", Focus: true, Panes: []Pane{{Dir: dir, Enter: true}}},
					{Name: "repo", Layout: "116d,95x51,0,0[95x34,0,0,1,95x16,0,35,16]", Panes: []Pane{
						{Dir: dir, Enter: true},
						{Dir: dir, Enter: true, Focus: true},
					}},
					{Name: "term", Panes: []Pane{{Dir: dir, Enter: true}}},
				},
			},
			// A window with nothing of its own is `{}`. Written as a bare `term:` it
			// would be a null scalar, which the reader turns into a pane sent an empty
			// line on every rebuild.
			want: `session: go-dev
dir: ~/main/go
windows:
    agent:
        focus: true
    repo:
        layout: 116d,95x51,0,0[95x34,0,0,1,95x16,0,35,16]
        0: {}
        1:
            focus: true
    term: {}
`,
		},
		{
			name: "a directory every pane shares moves up to the window",
			in: Session{
				Name: "s",
				Dir:  "/base",
				Windows: []Window{{Name: "w", Layout: "tiled", Panes: []Pane{
					{Dir: "/logs", Enter: true},
					{Dir: "/logs", Enter: true},
				}}},
			},
			want: `session: s
dir: /base
windows:
    w:
        dir: /logs
        layout: tiled
        0: {}
        1: {}
`,
		},
		{
			name: "panes that differ keep their own",
			in: Session{
				Name: "s",
				Dir:  "/base",
				Windows: []Window{{Name: "w", Layout: "tiled", Panes: []Pane{
					{Dir: "/base", Enter: true},
					{Dir: "/logs", Enter: true},
				}}},
			},
			want: `session: s
dir: /base
windows:
    w:
        layout: tiled
        0: {}
        1:
            dir: /logs
`,
		},
		{
			name: "keys, one line and several",
			in: Session{
				Name: "s",
				Windows: []Window{
					{Name: "one", Panes: []Pane{{Keys: []string{"ssh box"}, Enter: true}}},
					{Name: "typed", Panes: []Pane{{Keys: []string{"rm -rf /"}}}},
					{Name: "many", Panes: []Pane{{Keys: []string{"clear", "tail -f out.log"}, Enter: true}}},
				},
			},
			want: `session: s
windows:
    one:
        keys: ssh box
    typed:
        keys: rm -rf /
        enter: false
    many:
        keys:
            - clear
            - tail -f out.log
`,
		},
		{
			name: "a layout is dropped for a window that was never split",
			in: Session{
				Name:    "s",
				Windows: []Window{{Name: "w", Layout: "tiled", Panes: []Pane{{Enter: true}}}},
			},
			want: `session: s
windows:
    w: {}
`,
		},
		{
			name: "a name that would read back as something else is quoted",
			in: Session{
				Name:    "s",
				Windows: []Window{{Name: "0", Panes: []Pane{{Enter: true}}}, {Name: "true", Panes: []Pane{{Enter: true}}}},
			},
			want: `session: s
windows:
    "0": {}
    "true": {}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marshal(t, tt.in); got != tt.want {
				t.Errorf("Marshal =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestMarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		in   Session
		want string
	}{
		{"no name", Session{Windows: []Window{{Name: "w", Panes: []Pane{{}}}}}, "empty name"},
		{"no windows", Session{Name: "s"}, "no windows"},
		{"unnamed window", Session{Name: "s", Windows: []Window{{Panes: []Pane{{}}}}}, "window has an empty name"},
		{"duplicate window", Session{Name: "s", Windows: []Window{
			{Name: "w", Panes: []Pane{{}}}, {Name: "w", Panes: []Pane{{}}},
		}}, `duplicate window name "w"`},
		{"window with no panes", Session{Name: "s", Windows: []Window{{Name: "w"}}}, "no panes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Marshal(tt.in)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// The writer is only correct insofar as the reader agrees with it, and the directory
// collapse is where the two are easiest to drift apart: the reader pushes every
// inherited dir down onto the panes while the writer pulls them back up.
func TestRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	sessions := []Session{
		{
			Name: "captured",
			Dir:  filepath.Join(home, "main", "go"),
			Windows: []Window{
				{Name: "agent", Focus: true, Panes: []Pane{{Dir: filepath.Join(home, "main", "go"), Enter: true}}},
				{Name: "repo", Layout: "116d,95x51,0,0[95x34,0,0,1,95x16,0,35,16]", Panes: []Pane{
					{Dir: filepath.Join(home, "main", "go"), Enter: true},
					{Dir: filepath.Join(home, "logs"), Enter: true, Focus: true},
				}},
			},
		},
		{
			Name: "no session dir",
			Windows: []Window{
				{Name: "w", Panes: []Pane{{Keys: []string{"clear", "ls"}, Enter: true}}},
				{Name: "x", Panes: []Pane{{Keys: []string{"echo hi"}}}},
			},
		},
		{
			// A pane that overrides its window's directory cannot use the single-pane
			// form — the two share one `dir` key — so it has to come back numbered.
			Name:    "pane overrides its window",
			Dir:     "/base",
			Windows: []Window{{Name: "w", Dir: "/win", Panes: []Pane{{Dir: "/pane", Enter: true}}}},
		},
		{
			Name:    "awkward names",
			Windows: []Window{{Name: "0", Panes: []Pane{{Enter: true}}}, {Name: "no", Panes: []Pane{{Enter: true}}}},
		},
	}

	for _, want := range sessions {
		t.Run(want.Name, func(t *testing.T) {
			body := marshal(t, want)
			// Through a real file, so the filename-derived name and the tilde expansion
			// are both exercised the way `tmux_s` will hit them.
			got := mustParse(t, "written.yaml", body)
			if len(got) != 1 {
				t.Fatalf("got %d sessions, want 1\n%s", len(got), body)
			}
			if !reflect.DeepEqual(got[0], want) {
				t.Errorf("round trip =\n%#v\nwant\n%#v\nfrom\n%s", got[0], want, body)
			}
		})
	}
}
