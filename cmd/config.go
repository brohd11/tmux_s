package cmd

import (
	"github.com/brohd11/goutil/configcmd"
	"github.com/brohd11/tmux_s/internal/config"
)

func init() {
	rootCmd.AddCommand(configcmd.NewCommand(configcmd.Options{
		Path: config.Path,
		Dir:  config.Dir,
		Ensure: func() error {
			_, err := config.Ensure()
			return err
		},
	}))
}
