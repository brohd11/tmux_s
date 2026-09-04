package pathx

import (
	"os"
	"path/filepath"
	"testing"
)

// Expand backs the `dir` of both a session file and a sources list. It was two
// near-identical copies before; these are the cases that differed between them.
func TestExpand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("TMUX_S_TEST_DIR", "/somewhere")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"bare tilde is home", "~", home},
		{"tilde path", "~/projects", filepath.Join(home, "projects")},
		{"env var", "$TMUX_S_TEST_DIR/x", "/somewhere/x"},
		{"env and tilde together", "$TMUX_S_TEST_DIR", "/somewhere"},
		{"absolute is untouched", "/etc/hosts", "/etc/hosts"},
		{"relative is untouched", "sub/dir", "sub/dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Expand(tt.in); got != tt.want {
				t.Errorf("Expand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The empty-string guard is the one behavior the two old copies disagreed on: without
// it, "" expands to the home directory, which would silently start a session in $HOME
// instead of leaving tmux to use its own default.
func TestExpandEmptyIsNotHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if got := Expand(""); got != "" {
		t.Errorf(`Expand("") = %q, want ""`, got)
	}
}

// A "~user" form is not something strutil.ExpandHome will resolve. It comes back as
// written so tmux reports the real path in its own error, rather than the config load
// failing over a directory the user may not have meant to use yet.
func TestExpandOtherUserTildeIsReturnedAsWritten(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	const in = "~someoneelse/projects"
	if got := Expand(in); got != in {
		t.Errorf("Expand(%q) = %q, want it returned unchanged", in, got)
	}
}
