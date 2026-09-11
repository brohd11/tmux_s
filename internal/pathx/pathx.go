// Package pathx holds tmux_s's directory-string handling.
//
// It exists because Expand was written twice, near-verbatim, in internal/config and
// internal/spec -- both packages resolve a `dir` out of user-authored YAML and needed
// the same rules.
package pathx

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/goutil/strutil"
)

// Expand resolves $VAR and a leading ~ in a directory read from a session file or a
// sources list, so one file can be shared across machines whose home directories and
// environments differ. tmux's -c takes a literal path and does no expansion of its own,
// and nothing here goes through a shell that would.
//
// The tilde half is goutil/strutil.ExpandHome; the environment half is tmux_s's own
// addition on top. An empty string stays empty rather than becoming the home directory.
//
// A path that cannot be expanded is returned as it came: these strings are config, and
// surfacing the original in tmux's own "no such directory" error reads better than a
// half-resolved path or a startup failure over a directory the user may not have meant
// to use yet.
func Expand(p string) string {
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	expanded, err := strutil.ExpandHome(p)
	if err != nil {
		return p
	}
	return expanded
}

// Contract is Expand's inverse for the home directory half: a path under the user's home
// comes back as ~/…, so a file written by `tmux_s save` on one machine still resolves on
// another whose home directory sits elsewhere.
//
// Only the home prefix is undone. $VAR cannot be reversed — a value like /usr/local
// matches any number of variables and picking one would be a guess — so an environment
// variable the user wrote by hand is not restored, it is left as the path it expanded to.
//
// A path outside home, or one taken while the home directory cannot be determined, is
// returned unchanged: an absolute path is always correct, just less portable.
func Contract(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	// The separator has to be part of the match, or /home/bobby would contract against
	// a home of /home/bob and yield ~by.
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		// Written with forward slashes rather than the host separator. The tilde form
		// exists to be portable, and strutil.ExpandHome reads either spelling on either
		// OS, so a file saved on Windows still has to resolve on the machine it is
		// shared with.
		return "~/" + filepath.ToSlash(rest)
	}
	return p
}
