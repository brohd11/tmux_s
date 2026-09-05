package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/brohd11/goutil/configdir"
	"github.com/brohd11/tmux_s/internal/config"
	"github.com/brohd11/tmux_s/internal/spec"
	"github.com/brohd11/tmux_s/internal/tmux"

	"github.com/spf13/cobra"
)

var saveOverwrite bool

var saveCmd = &cobra.Command{
	Use:   "save <session> [name|path]",
	Short: "Write a running tmux session out as a session file",
	Long: `save — write a running tmux session out as a session file

Captures the shape of a session you built by hand: its windows and their order, the
names you gave them, the panes in each and the layout that places them, every pane's
working directory, and which window and pane are active.

It does not capture commands. tmux knows only what each pane is running now — a shell
when the pane is idle — never the line that was typed, so a captured session starts its
panes empty and any keys: worth keeping are added afterwards by hand.

  tmux_s save go-dev                     write ~/.tmux_s/sessions/go-dev.yaml
  tmux_s save go-dev backup              write it as the session backup
  tmux_s save go-dev ~/my-session.yaml   write it somewhere else
  tmux_s save go-dev --overwrite         replace the file that is already there

A bare second argument is a session name, not a file in the current directory: the copy
is renamed so 'tmux_s backup' builds it, instead of shadowing the session it came from. A
path only says where the file goes and leaves the name alone. A missing .yaml is added
either way, and an existing file is not overwritten without --overwrite.`,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
	RunE:         runSave,
}

func init() {
	// The root's --config is a local flag, not a persistent one, so it does not reach
	// here. Declaring it again on the command that needs it keeps it off `config` and
	// `update`, where a config file to read would mean nothing.
	f := saveCmd.Flags()
	f.StringVar(&configPath, "config", "", "config file to read (default ~/.tmux_s/config.yaml)")
	f.BoolVar(&saveOverwrite, "overwrite", false, "replace the destination file if it already exists")
	rootCmd.AddCommand(saveCmd)
}

func runSave(cmd *cobra.Command, args []string) error {
	name := args[0]
	override := ""
	if len(args) == 2 {
		override = args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	dest, err := cfg.Dest(name, override)
	if err != nil {
		return err
	}
	// Checked before tmux is touched, so a run that is going to refuse says so at once.
	// Overwriting is not the default because the file being replaced is the one with the
	// hand-added keys: in it, which a capture cannot put back — but it is a normal thing
	// to want once the session has been rearranged, hence the flag.
	if !saveOverwrite {
		if _, err := os.Stat(dest.Path); err == nil {
			return fmt.Errorf("%s already exists; pass --overwrite to replace it, or a different name or path", dest.Path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	session, err := tmux.Capture(name)
	if err != nil {
		return err
	}
	// A bare-name override renames the copy, so `tmux_s <that name>` builds it instead of
	// shadowing the session it came from.
	session.Name = dest.Session
	node, err := spec.Node(session)
	if err != nil {
		return err
	}
	if err := configdir.SaveAtomic(filepath.Dir(dest.Path), filepath.Base(dest.Path), node); err != nil {
		return err
	}

	if dest.Session != name {
		fmt.Fprintf(cmd.OutOrStdout(), "saved %s as %s -> %s\n", name, dest.Session, dest.Path)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "saved %s -> %s\n", name, dest.Path)
	return nil
}
