package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rally",
		Short:   "Project management sync — Jira, Linear, GitHub & Asana tickets as local markdown",
		Long:    "Rally syncs assigned tickets from Jira, Linear, GitHub, and Asana to local markdown files, so AI agents and humans can work from a local backlog without touching external APIs directly.",
		Version: version,
	}

	cmd.AddCommand(
		newConnectCmd(),
		newSyncCmd(),
		newListCmd(),
		newNextCmd(),
		newStartCmd(),
		newDoneCmd(),
		newStatusCmd(),
		newPinCmd(),
		newUnpinCmd(),
		newPinnedCmd(),
	)

	return cmd
}
