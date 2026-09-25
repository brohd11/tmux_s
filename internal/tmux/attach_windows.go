//go:build windows

package tmux

import (
	"os"
	"os/exec"
)

// execAttach on Windows runs tmux as a child (no exec); it exists so the package compiles.
func execAttach(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", exact(name))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
