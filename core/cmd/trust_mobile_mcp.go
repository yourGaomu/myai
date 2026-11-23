package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"myai/core/trust"
)

func newMobileMCPTrustCommand(options *trustOptions, testStore *trust.Store) *cobra.Command {
	command := &cobra.Command{Use: "mobile-admin", Short: "Manage exact pairing grants for existing HTTP MCP declarations"}
	var scope trust.MobileMCPScope
	var pairingID, endpoint string
	command.PersistentFlags().StringVar(&scope.UserID, "user", "local", "Agent user ID (same as agent --user)")
	command.PersistentFlags().StringVar(&scope.DeviceID, "device", "pc-local", "Agent device ID (same as agent --device)")
	command.PersistentFlags().StringVar(&pairingID, "pairing-id", "", "complete authorization ID from the selected paired client's Current entry")
	for _, action := range []string{"list", "grant", "revoke"} {
		child := &cobra.Command{
			Use: action, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				if info, err := os.Stat(options.workspace); err != nil || !info.IsDir() {
					return errors.New("workspace directory does not exist")
				}
				scope.Workspace = options.workspace
				var store *trust.MobileMCPStore
				if testStore != nil {
					store = trust.NewMobileMCPStore(filepath.Join(filepath.Dir(testStore.Path()), "mcp-mobile-admin.json"))
				} else {
					var err error
					store, err = trust.DefaultMobileMCPStore()
					if err != nil {
						return err
					}
				}
				switch action {
				case "list":
					return store.WithGrants(scope, pairingID, func(grants []trust.MobileMCPGrant) error {
						for _, grant := range grants {
							fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", grant.PairingID, grant.Endpoint)
						}
						return nil
					})
				case "grant":
					return store.Grant(scope, pairingID, endpoint)
				default:
					return store.Revoke(scope, pairingID)
				}
			},
		}
		if action == "grant" {
			child.Flags().StringVar(&endpoint, "public-endpoint", "", "exact resolved HTTP(S) URL (including its path); only specify an endpoint safe to show on the paired device")
			_ = child.MarkFlagRequired("public-endpoint")
		}
		command.AddCommand(child)
	}
	return command
}
