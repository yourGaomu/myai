package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"myai/core"
	"myai/core/remote/onebot"
)

var (
	onebotWSURL               string
	onebotToken               string
	onebotSuperAdmins         []int64
	onebotGroupAtOnly         bool
	onebotGroupSessionPerUser bool
	onebotCooldown            time.Duration
	onebotWorkspace           string
)

// onebotCmd 定义 `myai onebot` 子命令，用于启动对接 NapCatQQ (OneBot v11) 的 QQ 机器人服务。
// 1.1 初始化工作区与核心 Application 容器（加载配置、MongoDB、模型客户端与 ChatService）；
// 1.2 初始化 OneBot 持久化存储（有 MongoDB 时自动落库 `onebot_users` / `onebot_groups`，否则回退内存）；
// 1.3 创建 OneBot Bot 实例，并将 9 个自然语言管理与查询 Tool 动态挂载至全局工具注册表；
// 1.4 阻塞运行正向 WebSocket 事件循环，收到中断信号时优雅退出。
var onebotCmd = &cobra.Command{
	Use:   "onebot",
	Short: "Start myai OneBot v11 (NapCatQQ) QQ bot client",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1.1 设置工作区并启动核心 Application 容器
		core.SetWorkspace(onebotWorkspace)
		core.InitApp()
		app := core.GetApp()
		defer func() { _ = app.Close() }()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// 1.2 初始化 OneBot 身份与群配置存储（MongoDB / 内存双模自适应）
		store, err := onebot.NewStore(ctx, app.GetMongoDatabase())
		if err != nil {
			return err
		}

		// 1.3 构建 Bot 实例并将 9 个 OneBot 专属 Tool 注入工具注册表
		bot, err := onebot.NewBot(onebot.Config{
			WSURL:               onebotWSURL,
			Token:               onebotToken,
			SuperAdmins:         onebotSuperAdmins,
			GroupAtOnly:         onebotGroupAtOnly,
			GroupSessionPerUser: onebotGroupSessionPerUser,
			Cooldown:            onebotCooldown,
			Workspace:           onebotWorkspace,
		}, store, app.GetChatService())
		if err != nil {
			return err
		}

		if reg := app.GetToolRegister(); reg != nil {
			reg.RegisterSource("onebot", onebot.NewTools(store, app.GetChatService(), bot))
			defer reg.UnregisterSource("onebot")
		}

		// 1.4 启动正向 WebSocket 客户端主循环
		return bot.Run(ctx)
	},
}

// init 将 onebotCmd 注册至根命令并绑定命令行参数标志。
// 2.1 --ws-url：NapCatQQ 正向 WebSocket 地址（默认 ws://127.0.0.1:6700 或环境变量 MYAI_ONEBOT_WS_URL）；
// 2.2 --token：WebSocket 鉴权 Token（默认读取 MYAI_ONEBOT_TOKEN）；
// 2.3 --super-admin：种子超级管理员 QQ 号列表（支持多次指定或逗号分隔）；
// 2.4 --group-at-only / --group-session-per-user / --cooldown / --workspace：群聊策略与工作区配置。
func init() {
	rootCmd.AddCommand(onebotCmd)

	defaultWS := strings.TrimSpace(os.Getenv("MYAI_ONEBOT_WS_URL"))
	if defaultWS == "" {
		defaultWS = onebot.DefaultWSURL
	}

	onebotCmd.Flags().StringVar(&onebotWSURL, "ws-url", defaultWS, "NapCatQQ OneBot v11 forward websocket url")
	onebotCmd.Flags().StringVar(&onebotToken, "token", os.Getenv("MYAI_ONEBOT_TOKEN"), "OneBot v11 websocket access token")
	onebotCmd.Flags().Int64SliceVar(&onebotSuperAdmins, "super-admin", nil, "seed super_admin QQ user IDs (e.g. --super-admin 2823626561)")
	onebotCmd.Flags().BoolVar(&onebotGroupAtOnly, "group-at-only", true, "only reply in group chat when the bot is @mentioned")
	onebotCmd.Flags().BoolVar(&onebotGroupSessionPerUser, "group-session-per-user", false, "use isolated session per user inside group chats instead of shared group session")
	onebotCmd.Flags().DurationVar(&onebotCooldown, "cooldown", onebot.DefaultCooldown, "cooldown duration between messages for normal users")
	onebotCmd.Flags().StringVar(&onebotWorkspace, "workspace", ".", "workspace directory for local tools")
}
