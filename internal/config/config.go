// Package config finds the session files: it reads ~/.tmux_s/config.yaml for the list
// of source directories and indexes the YAML files they hold by session name.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/brohd11/goutil/configdir"
	"github.com/brohd11/tmux_s/internal/pathx"
	"github.com/brohd11/tmux_s/internal/spec"
)

// App is the name behind ~/.tmux_s — the hidden config directory convention every app
// in the monorepo shares (goutil/configdir).
const App = "tmux_s"

// Config is ~/.tmux_s/config.yaml.
//
// Sources is a list rather than a single directory so a dotfiles checkout can carry one
// set of sessions and the machine another: a laptop and a remote box share the repo but
// want different sessions, and listing both means neither has to be edited per machine.
type Config struct {
	Sources []string `yaml:"sources"`
}

// DefaultConfig is the explicit form of the implicit source SourceDirs uses. Writing
// it on `tmux_s config` keeps first-run behavior unchanged while putting the editable
// schema and its working default in front of the user.
func DefaultConfig() Config {
	return Config{Sources: []string{"~/.tmux_s/sessions"}}
}

// Entry is one session found on disk. File is kept so --list can say where a name came
// from, which is the only way to tell two same-named sessions apart.
type Entry struct {
	Session  spec.Session
	File     string
	Shadowed bool // a session of this name was already found in an earlier source
}

// Dir returns ~/.tmux_s.
func Dir() (string, error) { return configdir.Dir(App) }

// Path is ~/.tmux_s/config.yaml.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Ensure returns Path, materializing DefaultConfig when it is missing. An existing
// file is never rewritten, so opening the shared config command cannot disturb edits.
func Ensure() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := configdir.SaveAtomic(dir, "config.yaml", DefaultConfig()); err != nil {
		return "", err
	}
	return path, nil
}

// Load reads config.yaml. path overrides the default location; an empty path uses
// ~/.tmux_s/config.yaml. A missing file is not an error — it yields the default source
// list, which is the wanted first-run behavior.
func Load(path string) (Config, error) {
	var c Config
	if path == "" {
		var err error
		path, err = Path()
		if err != nil {
			return c, err
		}
	}
	if err := configdir.Load(path, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// SourceDirs is the list of directories to scan, expanded. With no sources configured
// it is ~/.tmux_s/sessions alone.
func (c Config) SourceDirs() ([]string, error) {
	if len(c.Sources) == 0 {
		dir, err := Dir()
		if err != nil {
			return nil, err
		}
		return []string{filepath.Join(dir, "sessions")}, nil
	}
	out := make([]string, 0, len(c.Sources))
	for _, s := range c.Sources {
		out = append(out, pathx.Expand(s))
	}
	return out, nil
}

// Scan reads every session defined under the configured sources, in source order.
//
// A source directory that does not exist is skipped silently: the whole point of the
// multi-source list is that the same config is shared by machines where only some of
// the paths are present. A directory that exists but cannot be read is reported —
// that is a real problem, not an absent machine.
//
// Names are first-wins, so an earlier source shadows a later one and a machine-local
// directory listed first can override the dotfiles copy. Shadowed entries are still
// returned, flagged, so --list can show them.
func Scan(dirs []string) ([]Entry, error) {
	var out []Entry
	seen := map[string]bool{}
	for _, dir := range dirs {
		files, err := sessionFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			sessions, err := spec.ParseFile(f)
			if err != nil {
				return nil, err
			}
			for _, s := range sessions {
				e := Entry{Session: s, File: f, Shadowed: seen[s.Name]}
				seen[s.Name] = true
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// Find returns the session with the given name, or a not-found error listing what is
// available — with no picker, the list is the discovery mechanism.
func Find(entries []Entry, name string) (Entry, error) {
	for _, e := range entries {
		if !e.Shadowed && e.Session.Name == name {
			return e, nil
		}
	}
	var names []string
	for _, e := range entries {
		if !e.Shadowed {
			names = append(names, e.Session.Name)
		}
	}
	if len(names) == 0 {
		return Entry{}, fmt.Errorf("no session named %q, and no sessions are defined", name)
	}
	sort.Strings(names)
	return Entry{}, fmt.Errorf("no session named %q; defined: %s", name, strings.Join(names, ", "))
}

// sessionFiles lists the .yaml/.yml files directly in dir, sorted. The scan is not
// recursive: a session directory is a flat drawer of files, and descending into it
// would make a nested checkout's own YAML look like session config.
func sessionFiles(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml":
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}
