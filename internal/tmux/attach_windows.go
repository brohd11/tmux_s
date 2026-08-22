//go:build windows

package tmux

import (
	"os"
	"os/exec"
)

// execAttach on Windows runs tmux as a child with the console inherited: there is no
// exec(2) to replace the process with. It exists so the package still compiles for a
// windows build, not because tmux runs there.
func execAttach(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", "="+name)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
