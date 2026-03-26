package cmd

import (
	"charm.land/bubbletea/v2"
	"dbf-sync/ui"

	"github.com/spf13/cobra"
)

var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Modo interactivo con menús",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := GetConfigPath(cmd)
		model := ui.NewAppModel(configPath, appVersion)
		p := tea.NewProgram(model)
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}