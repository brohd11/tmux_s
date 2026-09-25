//go:build !windows

package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// execAttach replaces this process with `tmux attach-session` so tmux owns the tty. It
// returns only on failure.
func execAttach(name string) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	argv := []string{"tmux", "attach-session", "-t", exact(name)}
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		return fmt.Errorf("attach %s: %w", name, err)
	}
	return nil
}
