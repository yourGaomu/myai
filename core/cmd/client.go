package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	remoteclient "myai/core/remote/client"
)

var (
	clientServerURL  string
	clientUserID     string
	clientDeviceID   string
	clientSessionID  string
	clientToken      string
	clientMessage    string
	clientAllowTools bool
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Start myai remote client simulator",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		c := remoteclient.New(remoteclient.Config{
			ServerURL:   clientServerURL,
			UserID:      clientUserID,
			DeviceID:    clientDeviceID,
			SessionID:   clientSessionID,
			ClientToken: clientToken,
			Message:     clientMessage,
			AllowTools:  clientAllowTools,
		})

		return c.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)

	clientCmd.Flags().StringVar(&clientServerURL, "server", "", "relay server websocket url")
	clientCmd.Flags().StringVar(&clientUserID, "user", "local", "user id")
	clientCmd.Flags().StringVar(&clientDeviceID, "device", "pc-local", "target agent device id")
	clientCmd.Flags().StringVar(&clientSessionID, "session", "", "session id")
	clientCmd.Flags().StringVar(&clientToken, "token", "", "client token from relay pairing")
	clientCmd.Flags().StringVar(&clientMessage, "message", "", "message to send")
	clientCmd.Flags().BoolVar(&clientAllowTools, "allow-tools", false, "allow remote tool requests")
}
