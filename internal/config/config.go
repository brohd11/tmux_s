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

// App is the name behind ~/.tmux_s.
const App = "tmux_s"

const configName = "config.yaml"

// Config is ~/.tmux_s/config.yaml. Sources is a list so shared dotfiles and machine-local
// sessions can both be scanned.
type Config struct {
	Sources []string `yaml:"sources"`
}

// DefaultConfig is the explicit form of the implicit default source.
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
	return filepath.Join(dir, configName), nil
}

// Ensure returns Path, materializing DefaultConfig when it is missing. An existing
// file is never rewritten, so opening the shared config command cannot disturb edits.
func Ensure() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if _, err := configdir.Ensure(dir, configName, DefaultConfig()); err != nil {
		return "", err
	}
	return filepath.Join(dir, configName), nil
}

// Load reads config.yaml (path overrides the default). A missing file yields the default.
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

// Scan reads every session under the sources, in order. Missing directories are skipped;
// unreadable ones are errors. Names are first-wins; shadowed entries are returned flagged.
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

// sessionFiles lists the .yaml/.yml files directly in dir, sorted (not recursive).
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

// Destination is where a save goes and under what name.
type Destination struct {
	Path string
	// Session is the name the written file declares. It is the captured session's own
	// name except where a bare-name override renamed it.
	Session string
}

// Dest is where `tmux_s save <session> [override]` writes. With no override:
// <first source>/<session>.yaml (the first source wins Scan). An override is:
//   - an existing directory -> <dir>/<session>.yaml;
//   - a bare word -> a new session name in the first source (a renamed copy);
//   - anything else -> that path, keeping the captured session's name.
//
// A missing .yaml extension is added.
func (c Config) Dest(session, override string) (Destination, error) {
	if override == "" {
		p, err := c.inSource(session)
		return Destination{Path: p, Session: session}, err
	}
	p := pathx.Expand(override)
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return Destination{Path: filepath.Join(p, withYAML(session)), Session: session}, nil
	}
	if !strings.ContainsAny(p, `/\`) {
		file := withYAML(p)
		// Same rule as spec: the basename without extension.
		name := strings.TrimSuffix(file, filepath.Ext(file))
		if name == "" {
			return Destination{}, fmt.Errorf("%q is not a usable session name", override)
		}
		path, err := c.inSource(file)
		return Destination{Path: path, Session: name}, err
	}
	return Destination{Path: withYAML(p), Session: session}, nil
}

// inSource places name in the first configured source directory.
func (c Config) inSource(name string) (string, error) {
	dirs, err := c.SourceDirs()
	if err != nil {
		return "", err
	}
	return filepath.Join(dirs[0], withYAML(name)), nil
}

// withYAML adds .yaml unless the path already ends in .yaml or .yml.
func withYAML(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".yaml", ".yml":
		return p
	}
	return p + ".yaml"
}
