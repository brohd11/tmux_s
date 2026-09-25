package tmux

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/brohd11/goutil/shellquote"
)

// exact is the -t target that matches session name exactly; a bare name is a prefix
// match, so "roblox" would find "roblox2".
func exact(name string) string { return "=" + name }

// Exists reports whether a session of this name is already running.
func Exists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", exact(name)).Run() == nil
}

// Build runs a plan, substituting captured pane ids, and stops at the first failure.
func Build(cmds []Command) error {
	var ids []string
	for _, c := range cmds {
		argv := resolve(c, ids)
		if c.Capture() {
			out, err := exec.Command("tmux", argv...).Output()
			if err != nil {
				return cmdError(argv, err)
			}
			ids = append(ids, strings.TrimSpace(string(out)))
			continue
		}
		if err := exec.Command("tmux", argv...).Run(); err != nil {
			return cmdError(argv, err)
		}
	}
	return nil
}

// Attach hands the terminal to the session: switch-client inside tmux, else execAttach.
func Attach(name string) error {
	if os.Getenv("TMUX") == "" {
		return execAttach(name)
	}
	argv := AttachCommand(name)
	if err := exec.Command("tmux", argv...).Run(); err != nil {
		return cmdError(argv, err)
	}
	return nil
}

// AttachCommand is the attach step Attach performs, as --print shows it.
func AttachCommand(name string) Command {
	if os.Getenv("TMUX") != "" {
		return Command{"switch-client", "-t", exact(name)}
	}
	return Command{"attach-session", "-t", exact(name)}
}

// Print writes the commands one per line, shell-quoted for reading only.
func Print(w io.Writer, cmds []Command) error {
	var ids []string
	next := 0
	for _, c := range cmds {
		if c.Capture() {
			// The ids are assigned at run time, so the preview shows a symbolic
			// %pane0, %pane1 … in the positions the real ids would take.
			ids = append(ids, fmt.Sprintf("%%pane%d", next))
			next++
		}
		if _, err := fmt.Fprintln(w, "tmux "+shellquote.JoinMinimal(resolve(c, ids))); err != nil {
			return err
		}
	}
	return nil
}

// resolve substitutes captured pane ids; an out-of-range ref is left as-is.
func resolve(c Command, ids []string) []string {
	out := make([]string, len(c))
	for i, a := range c {
		out[i] = a
		for n := len(ids) - 1; n >= 0; n-- {
			if a == paneRef(n) {
				out[i] = ids[n]
				break
			}
		}
	}
	return out
}

// cmdError names the failing command and prefers tmux's own message (captured in
// ExitError.Stderr by Output) to "exit status 1".
func cmdError(argv []string, err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		return fmt.Errorf("tmux %s: %s", shellquote.JoinMinimal(argv), strings.TrimSpace(string(exit.Stderr)))
	}
	return fmt.Errorf("tmux %s: %w", shellquote.JoinMinimal(argv), err)
}
