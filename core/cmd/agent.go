package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"myai/core"
	sqlitehistory "myai/core/adapter/persistence/sqlite/history"
	remoteagent "myai/core/remote/agent"
	"myai/core/remote/changes"
	"myai/core/remote/files"
)

var (
	agentServerURL string
	agentUserID    string
	agentDeviceID  string
	agentBindCode  string
	agentWorkspace string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start myai remote agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Agent 进程加载完整 AI 应用，并额外暴露当前 workspace 的文件与变更查询能力。
		core.SetWorkspace(agentWorkspace)
		core.InitApp()
		defer func() { _ = core.GetApp().Close() }()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		fileService, err := files.New(agentWorkspace)
		if err != nil {
			return err
		}
		changeService, err := changes.NewWithStoreFactory(agentWorkspace, "", sqlitehistory.Factory{})
		if err != nil {
			return err
		}

		// Relay 只接收这个窄 Facade，不直接持有 Application 或数据库对象。
		a := remoteagent.New(remoteagent.Config{
			ServerURL:   agentServerURL,
			UserID:      agentUserID,
			DeviceID:    agentDeviceID,
			BindingCode: agentBindCode,
			Workspace:   agentWorkspace,
		}, core.GetApp().GetChatService(), fileService, changeService)

		return a.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)

	agentCmd.Flags().StringVar(&agentServerURL, "server", "", "relay server websocket url")
	agentCmd.Flags().StringVar(&agentUserID, "user", "local", "user id")
	agentCmd.Flags().StringVar(&agentDeviceID, "device", "pc-local", "device id")
	agentCmd.Flags().StringVar(&agentBindCode, "bind-code", "", "fixed pairing code")
	agentCmd.Flags().StringVar(&agentWorkspace, "workspace", ".", "workspace directory for remote file preview and local tools")
}
