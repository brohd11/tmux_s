package tmux

import (
	"errors"
	"os/exec"
)

// asExitError is errors.As specialised to *exec.ExitError, kept apart so cmdError reads
// as the flow it is. exec.Command().Output() fills ExitError.Stderr, which carries
// tmux's own message ("can't find window: foo") — far more useful than "exit status 1".
func asExitError(err error, target **exec.ExitError) bool {
	return errors.As(err, target)
}
