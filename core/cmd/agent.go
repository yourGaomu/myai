package cmd

import (
	"context"
	"myai/core"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	remoteagent "myai/core/remote/agent"
)

var (
	agentServerURL string
	agentUserID    string
	agentDeviceID  string
	agentBindCode  string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start myai remote agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		core.InitApp()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		a := remoteagent.New(remoteagent.Config{
			ServerURL:   agentServerURL,
			UserID:      agentUserID,
			DeviceID:    agentDeviceID,
			BindingCode: agentBindCode,
		}, core.GetApp().GetChatService())

		return a.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)

	agentCmd.Flags().StringVar(&agentServerURL, "server", "", "relay server websocket url")
	agentCmd.Flags().StringVar(&agentUserID, "user", "local", "user id")
	agentCmd.Flags().StringVar(&agentDeviceID, "device", "pc-local", "device id")
	agentCmd.Flags().StringVar(&agentBindCode, "bind-code", "", "fixed pairing code")
}
