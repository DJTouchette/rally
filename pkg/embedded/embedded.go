// Package embedded exports rally's CLI command tree for embedding in other tools.
package embedded

import (
	"github.com/djtouchette/rally/internal/cli"
	"github.com/spf13/cobra"
)

// NewCommand returns rally's root cobra command.
// Callers can execute it directly or attach it as a subcommand.
func NewCommand(version string) *cobra.Command {
	return cli.NewRootCmd(version)
}
