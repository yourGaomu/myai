package cmd

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"myai/core"
	remoteagent "myai/core/remote/agent"
	"myai/core/remote/changes"
	"myai/core/remote/files"
)

var (
	oneBotURL       string
	oneBotToken     string
	oneBotUser      string
	oneBotDevice    string
	oneBotWorkspace string
)

var oneBotCmd = &cobra.Command{
	Use:   "one-bot",
	Short: "Start oneBot agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := ctx.Err(); err != nil {
			return err
		}
		// Agent 进程加载完整 AI 应用，并额外暴露当前 workspace 的文件与变更查询能力。
		core.SetWorkspace(oneBotWorkspace)
		core.InitApp()
		defer func() {
			if err := core.GetApp().Close(); err != nil {
				log.Printf("one-bot shutdown incomplete: %v", err)
			}
		}()

		fileService, err := files.New(oneBotWorkspace)
		if err != nil {
			return err
		}
		//记录修改历史记录和git diff类似的功能
		changeService, err := changes.NewWithStoreFactory(oneBotWorkspace, "", core.GetApp().HistoryStoreFactory())
		if err != nil {
			return err
		}

		// Relay 只接收这个窄 Facade，不直接持有 Application 或数据库对象。
		a := remoteagent.New(remoteagent.Config{
			ServerURL:  oneBotURL,
			RelayToken: oneBotToken,
			UserID:     oneBotUser,
			DeviceID:   oneBotDevice,
			Workspace:  oneBotWorkspace,
		}, core.GetApp().GetChatService(), fileService, changeService, core.GetApp().GetKnowledgeService(), core.GetApp().GetMemoryCatalogService(), core.GetApp().GetMemoryExtractionService(), core.GetApp().GetMemoryDreamService(), core.GetApp().GetSubagentService(), core.GetApp().GetSubagentEvents(), core.GetApp().GetPluginManager(),
			remoteagent.WithFileService(core.GetApp().GetFileService()),
			remoteagent.WithFileUploadAdmission(core.GetApp().GetChatService().AdmitFileUpload),
			remoteagent.WithFileAssetRecorder(core.GetApp().GetChatService().RecordUploadedAsset),
		)

		return a.WithMCPManager(core.GetApp()).Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(oneBotCmd)

	oneBotCmd.Flags().StringVar(&oneBotURL, "server", "", "relay server websocket url")
	oneBotCmd.Flags().StringVar(&oneBotToken, "relay-token", os.Getenv("MYAI_RELAY_AGENT_TOKEN"), "identity-bound token used to authenticate this agent to the relay")
	oneBotCmd.Flags().StringVar(&oneBotUser, "user", "local", "user id")
	oneBotCmd.Flags().StringVar(&oneBotDevice, "device", "onebot", "device id")
	oneBotCmd.Flags().StringVar(&oneBotWorkspace, "workspace", ".", "workspace directory for remote file preview and local tools")
}
