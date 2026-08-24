// Package pathx holds tmux_s's directory-string handling.
//
// It exists because Expand was written twice, near-verbatim, in internal/config and
// internal/spec -- both packages resolve a `dir` out of user-authored YAML and needed
// the same rules.
package pathx

import (
	"os"

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
