package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"myai/core"
	modelcommand "myai/core/application/model/command"
	sessionresult "myai/core/application/session/result"
	"myai/core/llm"
	"myai/core/service"
	"myai/core/skill"
)

var errModelAddCanceled = errors.New("model add canceled")

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an AI chat session",
	Run: func(cmd *cobra.Command, args []string) {
		runChat()
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func runChat() {
	// CLI 与手机 Agent 共用同一个 ChatService，只是输入输出适配器不同。
	core.InitApp()
	defer func() { _ = core.GetApp().Close() }()

	reader := bufio.NewScanner(os.Stdin)
	chatService := core.GetApp().GetChatService()
	ctx := context.Background()

	printChatHeader(chatService.CurrentSessionID(), chatService.CurrentModelID())

	for {
		printPrompt()

		if !reader.Scan() {
			fmt.Println()
			return
		}

		input := strings.TrimSpace(reader.Text())
		if input == "" {
			continue
		}

		// 斜杠命令直接调用会话管理能力，其余输入进入正常的流式生成链路。
		switch input {
		case "/exit", "exit", "quit":
			printSuccess("bye.")
			return
		case "/help":
			printChatHelp()
		case "/new":
			if err := chatService.NewSession(ctx); err != nil {
				printError("session error:", err)
				continue
			}
			printSuccess("new session: " + chatService.CurrentSessionID())
		case "/clear":
			if err := chatService.ClearCurrent(ctx); err != nil {
				printError("session error:", err)
				continue
			}
			printSuccess("current session cleared.")
		case "/sessions":
			printSessions(ctx, chatService)
		case "/skills":
			printSkills(ctx, chatService)
		case "/models":
			printModels(chatService)
		case "/model":
			printSuccess("current model: " + chatService.CurrentModelID())
		case "/permission":
			printPermissionMode(chatService.CurrentPermissionMode())
		case "/mode":
			printSuccess("current mode: " + string(chatService.CurrentAgentMode()))
		case "/context":
			printContextInfo(chatService.CurrentContextInfo())
		case "/compact":
			printWarning("compacting context...")
			info, err := chatService.CompactCurrentSession(ctx)
			if err != nil {
				printError("compact error:", err)
				continue
			}
			printSuccess("context compacted.")
			printContextInfo(info)
		case "/model add":
			if err := addModelInteractive(ctx, reader, chatService); err != nil {
				if errors.Is(err, errModelAddCanceled) {
					printWarning("model add canceled.")
					continue
				}
				printError("model add error:", err)
				continue
			}
			printModels(chatService)
		default:
			if strings.HasPrefix(input, "/permission ") {
				mode := strings.TrimSpace(strings.TrimPrefix(input, "/permission "))
				if err := chatService.SetPermissionMode(ctx, mode); err != nil {
					printError("permission error:", err)
					continue
				}
				printPermissionMode(chatService.CurrentPermissionMode())
				continue
			}

			if strings.HasPrefix(input, "/mode ") {
				mode := strings.TrimSpace(strings.TrimPrefix(input, "/mode "))
				if err := chatService.SetAgentMode(ctx, mode); err != nil {
					printError("mode error:", err)
					continue
				}
				printSuccess("current mode: " + string(chatService.CurrentAgentMode()))
				continue
			}

			if strings.HasPrefix(input, "/context ") {
				value := strings.TrimSpace(strings.TrimPrefix(input, "/context "))
				windowK, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(value), "k"))
				if err != nil {
					printWarning("usage: /context <K>, for example /context 16")
					continue
				}
				if err := chatService.SetContextWindowK(ctx, windowK); err != nil {
					printError("context error:", err)
					continue
				}
				printContextInfo(chatService.CurrentContextInfo())
				continue
			}

			if strings.HasPrefix(input, "/model ") {
				modelID := strings.TrimSpace(strings.TrimPrefix(input, "/model "))
				if err := chatService.SwitchModel(ctx, modelID); err != nil {
					printError("model error:", err)
					continue
				}
				printSuccess("current model: " + chatService.CurrentModelID())
				continue
			}

			if strings.HasPrefix(input, "/use ") {

				sessionID := strings.TrimSpace(strings.TrimPrefix(input, "/use "))
				if err := chatService.LoadSession(ctx, sessionID); err != nil {
					printError("session error:", err)
					continue
				}
				printSuccess("current session: " + chatService.CurrentSessionID())
				continue
			}

			printTurnDivider()
			printUserInput(input)
			printAssistantHeader()
			response, err := chatService.SendMessageStream(ctx, input, newChatStreamHandler(reader))
			if err != nil {
				fmt.Println()
				printError("AI error:", err)
				continue
			}
			printResponseFooter(response.SessionID, response.Result.Usage, response.Context, response.Compact)
		}
	}
}

func addModelInteractive(ctx context.Context, reader *bufio.Scanner, chatService interface {
	AddModelConfig(context.Context, modelcommand.AddConfig) error
}) error {
	printModelAddHeader()

	id, err := readModelField(reader, "id", "", true)
	if err != nil {
		return err
	}
	name, err := readModelField(reader, "name", id, false)
	if err != nil {
		return err
	}
	provider, err := readModelField(reader, "provider", "openai", false)
	if err != nil {
		return err
	}
	baseURL, err := readModelField(reader, "base url", "https://api.openai.com/v1", true)
	if err != nil {
		return err
	}
	apiKey, err := readModelField(reader, "api key (visible)", "", true)
	if err != nil {
		return err
	}
	modelName, err := readModelField(reader, "model name", id, false)
	if err != nil {
		return err
	}

	config := modelcommand.AddConfig{
		ID:        id,
		Name:      name,
		Provider:  provider,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		ModelName: modelName,
	}
	if err := chatService.AddModelConfig(ctx, config); err != nil {
		return err
	}

	printSuccess("model added: " + id)
	return nil
}

func readModelField(reader *bufio.Scanner, label string, defaultValue string, required bool) (string, error) {
	for {
		printFormPrompt(label, defaultValue)
		if !reader.Scan() {
			return "", errors.New("input closed")
		}

		value := strings.TrimSpace(reader.Text())
		if value == "/cancel" {
			return "", errModelAddCanceled
		}
		if value == "" {
			value = defaultValue
		}
		if value != "" || !required {
			return value, nil
		}

		printWarning(label + " is required.")
	}
}

func printSessions(ctx context.Context, chatService interface {
	ListSessions(context.Context) ([]sessionresult.SessionListItem, error)
	CurrentSessionID() string
}) {
	sessions, err := chatService.ListSessions(ctx)
	if err != nil {
		printError("session error:", err)
		return
	}
	printSessionsTable(sessions, chatService.CurrentSessionID())
}

func printSkills(ctx context.Context, chatService interface {
	ListSkills(context.Context) ([]skill.Skill, error)
	SkillRoot() string
}) {
	skills, err := chatService.ListSkills(ctx)
	if err != nil {
		printError("skill error:", err)
		return
	}
	printSkillsTable(skills, chatService.SkillRoot())
}

func printModels(chatService interface {
	ListModels() []llm.ModelInfo
	CurrentModelID() string
}) {
	printModelsTable(chatService.ListModels(), chatService.CurrentModelID())
}

func printContextInfo(info service.ContextInfo) {
	printContextWindow(info)
}
