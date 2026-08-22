// Command tmux_s builds a tmux session from a small YAML file and attaches to it.
//
// It replaces a bash router (~/.local/bin/tmux_s) that exec'd one hand-written script
// per session, each some twenty-five lines of the same has-session/new-window/send-keys
// boilerplate around a handful of strings that actually differed. The invocation is
// unchanged — `tmux_s roblox` — but a session is now a dozen lines of YAML.
//
// Nothing is generated and no shell is involved: the session file becomes a list of
// tmux argv slices run directly, and the final attach replaces this process with tmux.
package main

import "github.com/brohd11/tmux_s/cmd"

func main() {
	cmd.Execute()
}
