package tmux

// Capture reads a running session back into a spec.Session (layout and directories only).

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/brohd11/tmux_s/internal/spec"
)

// fieldSep is the ASCII unit separator, which neither window names nor paths contain. The
// path is the last field.
const fieldSep = "\x1f"

// paneFields are the list-panes fields Capture reads. Session values are read off the rows
// because display-message's -t doesn't take the =name form.
var paneFields = []string{
	"session_path",
	"window_index",
	"window_name",
	"window_active",
	"window_layout",
	"pane_index",
	"pane_active",
	"pane_current_path",
}

// Capture reads the running session name: windows, panes, layouts, directories and active
// window/pane. Not commands: tmux only knows the current process, so keys are left for the
// user to add.
func Capture(name string) (spec.Session, error) {
	if !Exists(name) {
		return spec.Session{}, fmt.Errorf("no session named %q is running", name)
	}

	// -s widens list-panes from the current window to every pane in the session, which it
	// reports window by window in index order.
	argv := []string{"list-panes", "-s", "-t", exact(name), "-F", format()}
	out, err := exec.Command("tmux", argv...).Output()
	if err != nil {
		return spec.Session{}, cmdError(argv, err)
	}

	s, err := parsePanes(name, string(out))
	if err != nil {
		return spec.Session{}, fmt.Errorf("session %q: %w", name, err)
	}
	return s, nil
}

// format is the -F string for paneFields.
func format() string {
	parts := make([]string, len(paneFields))
	for i, f := range paneFields {
		parts[i] = "#{" + f + "}"
	}
	return strings.Join(parts, fieldSep)
}

// parsePanes turns list-panes output into a session. It is separate from the exec so the
// mapping can be tested against fixed rows, with no tmux to run.
func parsePanes(name, out string) (spec.Session, error) {
	s := spec.Session{Name: name}

	type pending struct {
		index int
		pane  spec.Pane
	}
	var panes [][]pending // one bucket per window, in the order the windows first appear
	byWindow := map[string]int{}
	seenName := map[string]bool{}

	for n, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, fieldSep)
		if len(f) != len(paneFields) {
			return s, fmt.Errorf("line %d: expected %d fields, got %d: %q", n+1, len(paneFields), len(f), line)
		}
		s.Dir = f[0] // the same on every row; the session has one working directory
		windowIndex, windowName, windowActive, layout := f[1], f[2], f[3], f[4]
		paneIndex, paneActive, paneDir := f[5], f[6], f[7]

		pi, err := strconv.Atoi(paneIndex)
		if err != nil {
			return s, fmt.Errorf("line %d: bad pane index %q: %w", n+1, paneIndex, err)
		}

		w, ok := byWindow[windowIndex]
		if !ok {
			if windowName == "" {
				return s, fmt.Errorf("window %s has no name", windowIndex)
			}
			// Windows are addressed by name on rebuild, so duplicates are an error.
			if seenName[windowName] {
				return s, fmt.Errorf("two windows are named %q; rename one, windows are matched by name when the session is rebuilt", windowName)
			}
			seenName[windowName] = true
			byWindow[windowIndex] = len(s.Windows)
			w = len(s.Windows)
			s.Windows = append(s.Windows, spec.Window{
				Name:   windowName,
				Layout: layout,
				Focus:  windowActive == "1",
			})
			panes = append(panes, nil)
		}
		panes[w] = append(panes[w], pending{
			index: pi,
			pane: spec.Pane{
				Dir:   paneDir,
				Enter: true, // the reader's default; nothing is sent, but the structs must match what parsing one back yields
				Focus: paneActive == "1",
			},
		})
	}

	if len(s.Windows) == 0 {
		return s, fmt.Errorf("no panes reported")
	}

	for i := range s.Windows {
		p := panes[i]
		// tmux lists panes in index order already; sorting makes that a property of this
		// function rather than of the command it happens to be fed.
		sort.SliceStable(p, func(a, b int) bool { return p[a].index < p[b].index })
		// Drop the indices (they depend on pane-base-index); order carries over.
		for _, e := range p {
			s.Windows[i].Panes = append(s.Windows[i].Panes, e.pane)
		}
		// Focus and layout only mean something where there is a choice of pane. On a
		// single pane both are noise the planner would ignore.
		if len(p) < 2 {
			s.Windows[i].Layout = ""
			s.Windows[i].Panes[0].Focus = false
		}
	}
	return s, nil
}
