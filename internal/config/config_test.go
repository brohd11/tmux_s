package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A source directory that isn't there is skipped, not reported: the whole reason
// sources is a list is that one config is shared by machines where only some of the
// paths exist.
func TestScanSkipsMissingSources(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.yaml", "windows:\n  w: x\n")
	got, err := Scan([]string{filepath.Join(dir, "nope"), dir, "/also/not/here"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].Session.Name != "a" {
		t.Fatalf("got %v, want the one session from the real directory", got)
	}
}

// Earlier sources win, so a machine-local directory listed first overrides the shared
// dotfiles copy. The loser is kept and flagged so --list can show it.
func TestScanFirstSourceWins(t *testing.T) {
	local, shared := t.TempDir(), t.TempDir()
	write(t, local, "roblox.yaml", "windows:\n  local: x\n")
	write(t, shared, "roblox.yaml", "windows:\n  shared: y\n")

	got, err := Scan([]string{local, shared})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want both kept", len(got))
	}
	if got[0].Shadowed || !got[1].Shadowed {
		t.Errorf("shadowing = %v/%v, want the second entry marked", got[0].Shadowed, got[1].Shadowed)
	}
	found, err := Find(got, "roblox")
	if err != nil {
		t.Fatal(err)
	}
	if found.Session.Windows[0].Name != "local" {
		t.Errorf("Find returned the %q window, want the one from the first source", found.Session.Windows[0].Name)
	}
}

// Only .yaml/.yml at the top level; a nested checkout's own YAML is not session config.
func TestScanIgnoresOtherFilesAndSubdirectories(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.yaml", "windows:\n  w: x\n")
	write(t, dir, "b.yml", "windows:\n  w: x\n")
	write(t, dir, "old.sh", "#!/usr/bin/env bash\n")
	write(t, dir, "README.md", "notes")
	write(t, filepath.Join(dir, "nested"), "c.yaml", "windows:\n  w: x\n")

	got, err := Scan([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Session.Name)
	}
	if strings.Join(names, ",") != "a,b" {
		t.Errorf("sessions = %v, want [a b]", names)
	}
}

// The not-found error has to carry the list, since there is no picker to browse.
func TestFindNotFoundNamesWhatExists(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go-dev.yaml", "windows:\n  w: x\n")
	entries, err := Scan([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Find(entries, "roblox")
	if err == nil || !strings.Contains(err.Error(), "go-dev") {
		t.Errorf("error = %v, want it to list go-dev", err)
	}
}

// A parse error names its file rather than failing anonymously somewhere in the scan.
func TestScanReportsABadFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.yaml", "windows:\n  w:\n    nope: 1\n")
	if _, err := Scan([]string{dir}); err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("error = %v, want it to name broken.yaml", err)
	}
}

// With nothing configured the default is ~/.tmux_s/sessions.
func TestSourceDirsDefault(t *testing.T) {
	got, err := Config{}.SourceDirs()
	if err != nil {
		t.Skip("no home directory")
	}
	if len(got) != 1 || filepath.Base(got[0]) != "sessions" || filepath.Base(filepath.Dir(got[0])) != "."+App {
		t.Errorf("default sources = %v, want ~/.tmux_s/sessions", got)
	}
}

// Sources are expanded, so a dotfiles config can say ~/dotfiles/... on every machine.
func TestSourceDirsExpand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	got, err := Config{Sources: []string{"~/dotfiles/tmux_sessions"}}.SourceDirs()
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != filepath.Join(home, "dotfiles/tmux_sessions") {
		t.Errorf("expanded to %q", got[0])
	}
}

// A missing config.yaml is the first-run case, not a failure.
func TestLoadMissingFile(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Sources) != 0 {
		t.Errorf("sources = %v, want none", got.Sources)
	}
}
