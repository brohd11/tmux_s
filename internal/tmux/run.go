package tmux

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Exists reports whether a session of this name is already running.
//
// The =name form forces an exact match. Without it tmux treats the target as a prefix,
// so `tmux_s roblox` would find an unrelated `roblox2` and attach to that instead of
// building the session asked for.
func Exists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", "="+name).Run() == nil
}

// Build runs a plan, substituting each captured pane id into the commands that refer to
// it. It stops at the first failure: a half-built session is not something to keep
// piling windows onto, and the error names the command that broke.
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

// Attach hands the terminal to the session.
//
// Inside tmux this has to be switch-client: attach-session from within a session
// refuses ("sessions should be nested with care"). Outside it, execAttach replaces this
// process with tmux so the client owns the tty directly, which is what the bash
// scripts' trailing `tmux attach-session` did.
func Attach(name string) error {
	if os.Getenv("TMUX") != "" {
		argv := []string{"switch-client", "-t", "=" + name}
		if err := exec.Command("tmux", argv...).Run(); err != nil {
			return cmdError(argv, err)
		}
		return nil
	}
	return execAttach(name)
}

// AttachCommand is the attach step as it appears in --print, so the printed plan is the
// whole story rather than everything up to the interesting part.
func AttachCommand(name string) Command {
	if os.Getenv("TMUX") != "" {
		return Command{"switch-client", "-t", "=" + name}
	}
	return Command{"attach-session", "-t", "=" + name}
}

// Print writes the commands as they would be run, one per line, with arguments quoted
// only where a shell would need it. It is a rendering for reading — the real run never
// builds a command line.
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
		if _, err := fmt.Fprintln(w, "tmux "+shellJoin(resolve(c, ids))); err != nil {
			return err
		}
	}
	return nil
}

// resolve replaces the plan's pane placeholders with the ids captured so far. A
// reference to a pane that has not been created yet cannot occur in a plan Plan built,
// so an out-of-range ref is left as-is rather than papered over — it would surface as a
// tmux "can't find pane" naming the placeholder.
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

// cmdError names the failing command and prefers tmux's own message to "exit status 1".
func cmdError(argv []string, err error) error {
	var exit *exec.ExitError
	if asExitError(err, &exit) && len(exit.Stderr) > 0 {
		return fmt.Errorf("tmux %s: %s", shellJoin(argv), strings.TrimSpace(string(exit.Stderr)))
	}
	return fmt.Errorf("tmux %s: %w", shellJoin(argv), err)
}

// shellJoin renders an argv the way a shell would have to be given it. Display only —
// see the Command doc comment; a real run passes these as separate arguments and never
// builds a command line at all.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, needsQuote) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func needsQuote(r rune) bool {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return false
	case r == '-', r == '_', r == '.', r == '/', r == ':', r == '=', r == '%', r == '+', r == ',', r == '@':
		return false
	}
	return true
}
