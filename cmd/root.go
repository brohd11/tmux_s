package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/brohd11/tmux_s/internal/config"
	"github.com/brohd11/tmux_s/internal/tmux"

	"github.com/spf13/cobra"
)

// version is the binary version; defaults to "dev" for a plain `go build`. The makefile
// stamps it via -X ldflags (git describe --tags --always --dirty), so release and `make`
// binaries report their real version and the self-update check can compare it against
// the latest tag.
var version = "dev"

var (
	configPath string
	list       bool
	printOnly  bool
	detach     bool
)

var rootCmd = &cobra.Command{
	Use:   "tmux_s [session]",
	Short: "Build a tmux session from a YAML file and attach to it",
	Long: `tmux_s — build a tmux session from a YAML file and attach to it

Session files are the *.yaml under the directories listed in ~/.tmux_s/config.yaml
(default ~/.tmux_s/sessions). A session that is already running is attached to as-is,
never rebuilt.

  tmux_s                  list the sessions that are defined
  tmux_s roblox           build (or attach to) the session named roblox
  tmux_s roblox --print    show the tmux commands without running any
  tmux_s roblox --detach   build the session but stay where you are
  tmux_s update            update to the latest release`,
	Version:       version,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runRoot,
}

func init() {
	rootCmd.SetVersionTemplate("tmux_s {{.Version}}\n")
	f := rootCmd.Flags()
	f.StringVar(&configPath, "config", "", "config file to read (default ~/.tmux_s/config.yaml)")
	f.BoolVarP(&list, "list", "l", false, "list the defined sessions and the files they come from")
	f.BoolVar(&printOnly, "print", false, "print the tmux commands that would run; run nothing")
	f.BoolVarP(&detach, "detach", "d", false, "build the session without attaching to it")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	dirs, err := cfg.SourceDirs()
	if err != nil {
		return err
	}
	entries, err := config.Scan(dirs)
	if err != nil {
		return err
	}

	// No session named: show what there is. The bash router printed "Provide session id
	// as arg", which told you nothing you did not already know.
	if len(args) == 0 || list {
		return printList(cmd.OutOrStdout(), entries, dirs)
	}

	name := args[0]
	entry, err := config.Find(entries, name)
	if err != nil {
		return err
	}
	plan := tmux.Plan(entry.Session)

	if printOnly {
		out := cmd.OutOrStdout()
		if tmux.Exists(name) {
			// Nothing would be built, so printing the build would be a lie about what
			// the same invocation does right now.
			fmt.Fprintf(out, "# session %q already exists; only the attach would run\n", name)
		} else if err := tmux.Print(out, plan); err != nil {
			return err
		}
		if detach {
			return nil
		}
		return tmux.Print(out, []tmux.Command{tmux.AttachCommand(name)})
	}

	if !tmux.Exists(name) {
		if err := tmux.Build(plan); err != nil {
			return err
		}
	}
	if detach {
		return nil
	}
	return tmux.Attach(name)
}

// printList writes the defined sessions, with the file each came from. Shadowed names —
// a second definition in a later source — are marked rather than hidden, since a
// session silently not being the one you wrote is exactly the confusing case.
//
// The sources are echoed when there is nothing to list: with directories that are
// skipped when absent by design, "no sessions" otherwise gives no hint about where
// tmux_s looked.
func printList(out io.Writer, entries []config.Entry, dirs []string) error {
	if len(entries) == 0 {
		fmt.Fprintln(out, "no sessions defined. searched:")
		for _, d := range dirs {
			fmt.Fprintln(out, "  "+d)
		}
		return nil
	}
	sorted := make([]config.Entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Session.Name < sorted[j].Session.Name })

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, e := range sorted {
		note := ""
		if e.Shadowed {
			note = "\t(shadowed)"
		}
		fmt.Fprintf(tw, "%s\t%d window(s)\t%s%s\n", e.Session.Name, len(e.Session.Windows), e.File, note)
	}
	return tw.Flush()
}
