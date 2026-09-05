package tmux

import (
	"reflect"
	"strings"
	"testing"

	"github.com/brohd11/tmux_s/internal/spec"
)

// row builds one list-panes line in the order paneFields declares. The session path leads
// every row, repeated, the same way tmux repeats it.
func row(sessionDir, windowIndex, windowName, windowActive, layout, paneIndex, paneActive, paneDir string) string {
	return strings.Join([]string{sessionDir, windowIndex, windowName, windowActive, layout, paneIndex, paneActive, paneDir}, fieldSep)
}

func TestFormat(t *testing.T) {
	want := "#{session_path}\x1f#{window_index}\x1f#{window_name}\x1f#{window_active}\x1f#{window_layout}\x1f#{pane_index}\x1f#{pane_active}\x1f#{pane_current_path}"
	if got := format(); got != want {
		t.Errorf("format() = %q, want %q", got, want)
	}
}

func TestParsePanes(t *testing.T) {
	tests := []struct {
		name string
		rows []string
		dir  string
		want []spec.Window
	}{
		{
			name: "single pane windows",
			dir:  "/home/u/go",
			rows: []string{
				row("/home/u/go", "0", "agent", "1", "af1d,95x51,0,0,0", "0", "1", "/home/u/go"),
				row("/home/u/go", "1", "term", "0", "af1f,95x51,0,0,2", "0", "1", "/home/u/go"),
			},
			want: []spec.Window{
				// The layout and the pane's focus are dropped: with one pane there is
				// nothing for either to select.
				{Name: "agent", Focus: true, Panes: []spec.Pane{{Dir: "/home/u/go", Enter: true}}},
				{Name: "term", Panes: []spec.Pane{{Dir: "/home/u/go", Enter: true}}},
			},
		},
		{
			name: "split window keeps its layout and active pane",
			dir:  "/home/u/go",
			rows: []string{
				row("/home/u/go", "0", "repo", "0", "116d,95x51,0,0[95x34,0,0,1,95x16,0,35,16]", "0", "0", "/home/u/go"),
				row("/home/u/go", "0", "repo", "0", "116d,95x51,0,0[95x34,0,0,1,95x16,0,35,16]", "1", "1", "/home/u/go/gote"),
			},
			want: []spec.Window{{
				Name:   "repo",
				Layout: "116d,95x51,0,0[95x34,0,0,1,95x16,0,35,16]",
				Panes: []spec.Pane{
					{Dir: "/home/u/go", Enter: true},
					{Dir: "/home/u/go/gote", Enter: true, Focus: true},
				},
			}},
		},
		{
			name: "pane order follows the index, not the row order",
			dir:  "/home/u",
			rows: []string{
				row("/home/u", "0", "w", "1", "L", "2", "0", "/c"),
				row("/home/u", "0", "w", "1", "L", "0", "0", "/a"),
				row("/home/u", "0", "w", "1", "L", "1", "0", "/b"),
			},
			want: []spec.Window{{
				Name: "w", Layout: "L", Focus: true,
				Panes: []spec.Pane{
					{Dir: "/a", Enter: true},
					{Dir: "/b", Enter: true},
					{Dir: "/c", Enter: true},
				},
			}},
		},
		{
			name: "base-index and pane-base-index are not carried over",
			dir:  "/home/u",
			rows: []string{
				row("/home/u", "1", "one", "0", "L", "1", "0", "/a"),
				row("/home/u", "1", "one", "0", "L", "2", "1", "/b"),
				row("/home/u", "2", "two", "1", "M", "1", "1", "/c"),
			},
			want: []spec.Window{
				{Name: "one", Layout: "L", Panes: []spec.Pane{
					{Dir: "/a", Enter: true},
					{Dir: "/b", Enter: true, Focus: true},
				}},
				{Name: "two", Focus: true, Panes: []spec.Pane{{Dir: "/c", Enter: true}}},
			},
		},
		{
			name: "a name with a tab in it survives the split",
			dir:  "/home/u",
			rows: []string{row("/home/u", "0", "a\tb", "1", "L", "0", "1", "/x\ty")},
			want: []spec.Window{{Name: "a\tb", Focus: true, Panes: []spec.Pane{{Dir: "/x\ty", Enter: true}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePanes("s", strings.Join(tt.rows, "\n")+"\n")
			if err != nil {
				t.Fatalf("parsePanes: %v", err)
			}
			if got.Name != "s" || got.Dir != tt.dir {
				t.Errorf("session = %q %q, want %q %q", got.Name, got.Dir, "s", tt.dir)
			}
			if !reflect.DeepEqual(got.Windows, tt.want) {
				t.Errorf("windows =\n%#v\nwant\n%#v", got.Windows, tt.want)
			}
		})
	}
}

func TestParsePanesErrors(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "duplicate window name",
			out: strings.Join([]string{
				row("/home/u", "0", "dev", "1", "L", "0", "1", "/a"),
				row("/home/u", "1", "dev", "0", "L", "0", "1", "/b"),
			}, "\n"),
			want: `two windows are named "dev"`,
		},
		{
			name: "short row",
			out:  "/home/u\x1f0\x1fw\x1f1",
			want: "expected 8 fields, got 4",
		},
		{
			name: "bad pane index",
			out:  row("/home/u", "0", "w", "1", "L", "x", "1", "/a"),
			want: `bad pane index "x"`,
		},
		{
			name: "unnamed window",
			out:  row("/home/u", "0", "", "1", "L", "0", "1", "/a"),
			want: "window 0 has no name",
		},
		{
			name: "no panes",
			out:  "\n\n",
			want: "no panes reported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePanes("s", tt.out)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}
