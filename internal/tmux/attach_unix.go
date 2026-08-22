//go:build !windows

package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// execAttach replaces this process with `tmux attach-session`, so tmux inherits the
// terminal outright rather than running as a child with tmux_s waiting on it. That is
// what the bash scripts did by ending in `tmux attach-session`, and it matters: a tmux
// client wants the tty, and an intervening parent shows up in job control and signal
// handling for the whole life of the session.
//
// It returns only on failure — on success this process is gone.
func execAttach(name string) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	argv := []string{"tmux", "attach-session", "-t", "=" + name}
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		return fmt.Errorf("attach %s: %w", name, err)
	}
	return nil
}
