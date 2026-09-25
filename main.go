// Command tmux_s builds a tmux session from a small YAML file and attaches to it. The file
// becomes tmux argv run directly (no shell), and the attach replaces this process.
package main

import "github.com/brohd11/tmux_s/cmd"

func main() {
	cmd.Execute()
}
