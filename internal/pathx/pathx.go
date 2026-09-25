// Package pathx expands and contracts directory strings from session YAML.
package pathx

import (
	"os"
	"path/filepath"

	"github.com/brohd11/goutil/strutil"
)

// Expand resolves $VAR and a leading ~ (tmux -c does no expansion). Empty stays empty; a
// path that cannot be expanded is returned unchanged.
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

// Contract turns a path under home back into ~/… so saved files are portable. $VAR is not
// restored.
func Contract(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	c := strutil.ContractHome(p, home)
	if c == p {
		return p
	}
	// Forward slashes: the tilde form exists to be portable, and ExpandHome reads either.
	return filepath.ToSlash(c)
}
