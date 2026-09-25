// Package tmux turns a parsed session into tmux commands and runs them. Plan is pure, so
// --print and the tests need no tmux.
package tmux

import (
	"fmt"

	"github.com/brohd11/tmux_s/internal/spec"
)

// Command is one tmux argv without the leading "tmux". No shell is involved.
type Command []string

// Capture reports whether the command prints a pane id (-P -F '#{pane_id}') for later
// commands.
func (c Command) Capture() bool {
	if len(c) == 0 {
		return false
	}
	switch c[0] {
	case "new-session", "new-window", "split-window":
		return true
	}
	return false
}

// paneRef is a placeholder for a pane id known only at run time. Ids are independent of
// base-index and renumbering.
func paneRef(n int) string { return fmt.Sprintf("\x00pane%d\x00", n) }

// Plan returns the commands that build s, excluding the attach. Order matters: session
// before windows, windows before splits, select-layout after the splits.
func Plan(s spec.Session) []Command {
	var cmds []Command
	pane := 0 // next pane ref to hand out, in creation order
	focusWindow := s.Windows[0].Name
	windowFocused := false

	for wi, w := range s.Windows {
		first := pane
		if wi == 0 {
			cmds = append(cmds, create(Command{"new-session", "-d", "-s", s.Name, "-n", w.Name}, w.Panes[0].Dir, s.Dir))
		} else {
			cmds = append(cmds, create(Command{"new-window", "-t", s.Name + ":", "-n", w.Name}, w.Panes[0].Dir, s.Dir))
		}
		pane++

		// Each split targets the pane created just before it, so the geometry follows
		// the file's order instead of whichever pane tmux happens to have active.
		for pi := 1; pi < len(w.Panes); pi++ {
			cmds = append(cmds, create(Command{"split-window", "-t", paneRef(pane - 1)}, w.Panes[pi].Dir, s.Dir))
			pane++
		}

		if len(w.Panes) > 1 {
			layout := w.Layout
			if layout == "" {
				layout = "tiled"
			}
			cmds = append(cmds, Command{"select-layout", "-t", target(s.Name, w.Name), layout})
		}

		for pi, p := range w.Panes {
			for _, line := range p.Keys {
				c := Command{"send-keys", "-t", paneRef(first + pi), line}
				if p.Enter {
					c = append(c, "C-m")
				}
				cmds = append(cmds, c)
			}
		}

		// Select the first pane explicitly; splitting leaves the last one active.
		if len(w.Panes) > 1 {
			focusPane := first
			for pi, p := range w.Panes {
				if p.Focus {
					focusPane = first + pi
					break
				}
			}
			cmds = append(cmds, Command{"select-pane", "-t", paneRef(focusPane)})
		}

		if w.Focus && !windowFocused {
			focusWindow = w.Name
			windowFocused = true
		}
	}

	cmds = append(cmds, Command{"select-window", "-t", target(s.Name, focusWindow)})
	return cmds
}

// create adds the directory and -P -F so tmux prints the new pane's id.
func create(c Command, dir, sessionDir string) Command {
	return append(withDir(c, dir, sessionDir), "-P", "-F", "#{pane_id}")
}

// withDir appends -c dir, falling back to the session's directory.
func withDir(c Command, dir, sessionDir string) Command {
	if dir == "" {
		dir = sessionDir
	}
	if dir == "" {
		return c
	}
	return append(c, "-c", dir)
}

// target names a window as <session>:<window name> — never by index. Window indices
// shift under base-index and renumber-windows; a name does not.
func target(session, window string) string { return session + ":" + window }
