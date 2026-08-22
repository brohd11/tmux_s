// Package tmux turns a parsed session into tmux invocations and runs them.
//
// The plan and the run are separate on purpose: Plan is a pure function from a spec to
// a list of argv slices, so --print is a loop over its output and the whole command
// shape is testable without a tmux on the machine.
package tmux

import (
	"fmt"

	"github.com/brohd11/tmux_s/internal/spec"
)

// Command is one tmux invocation as an argv, without the leading "tmux".
//
// It stays an argv rather than a string because nothing here goes through a shell: a
// send-keys payload full of quotes, && and $ is one element, and the escaping the bash
// scripts had to get right does not arise.
type Command []string

// Capture reports whether a command prints a pane id the plan needs to substitute into
// later commands. new-session, new-window and split-window are run with
// -P -F '#{pane_id}' for exactly that.
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

// paneRef is the placeholder a plan carries where a pane id will go. The ids are only
// known at run time (tmux assigns %0, %1, …), so Plan emits refs and the runner
// rewrites them with the ids it captured.
//
// Referring to panes by id rather than by index is what makes the plan immune to a
// user's base-index or renumber-windows settings: %3 names one pane for as long as it
// exists, while :1.0 means different things in different configs.
func paneRef(n int) string { return fmt.Sprintf("\x00pane%d\x00", n) }

// Plan is the full sequence of tmux commands that builds s, excluding the attach.
//
// Order matters throughout: the session must exist before its second window, a window
// before its splits, and select-layout after every split in that window (it rebalances
// what is there when it runs).
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

		// select-pane runs after the splits, since splitting leaves the new pane
		// active. Without a marked pane the window would keep tmux's default, which is
		// the last one created — so the first pane is selected explicitly. A window
		// with one pane needs none of this.
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

// create finishes one of the three pane-creating commands: the working directory, then
// the -P -F pair that makes tmux print the new pane's id. The id is what every later
// command in the plan targets, so it is captured even when nothing visibly needs it.
func create(c Command, dir, sessionDir string) Command {
	return append(withDir(c, dir, sessionDir), "-P", "-F", "#{pane_id}")
}

// withDir appends -c when a directory is set, falling back to the session's. tmux takes
// the flag on new-session, new-window and split-window alike, so one helper covers all
// three.
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
