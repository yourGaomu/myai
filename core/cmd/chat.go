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
	generation "myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
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
		case "/generation":
			preferences, err := chatService.CurrentSessionPreferences(ctx)
			if err != nil {
				printError("generation settings error:", err)
				continue
			}
			printSessionPreferences(preferences)
		case "/generation reset":
			if err := chatService.SetGenerationSettings(ctx, generation.Settings{}); err != nil {
				printError("generation settings error:", err)
				continue
			}
			printSuccess("session generation settings reset to model defaults.")
		case "/style":
			preferences, err := chatService.CurrentSessionPreferences(ctx)
			if err != nil {
				printError("style error:", err)
				continue
			}
			printStyleInstruction(preferences.StyleInstruction)
		case "/style reset":
			if err := chatService.SetStyleInstruction(ctx, ""); err != nil {
				printError("style error:", err)
				continue
			}
			printSuccess("session style reset.")
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
			if strings.HasPrefix(input, "/generation set ") {
				settings, err := parseGenerationSettings(input)
				if err != nil {
					printWarning(err.Error())
					continue
				}
				if err := chatService.SetGenerationSettings(ctx, settings); err != nil {
					printError("generation settings error:", err)
					continue
				}
				printSuccess("session generation settings updated.")
				continue
			}

			if strings.HasPrefix(input, "/style ") {
				instruction := strings.TrimSpace(strings.TrimPrefix(input, "/style "))
				if err := chatService.SetStyleInstruction(ctx, instruction); err != nil {
					printError("style error:", err)
					continue
				}
				printSuccess("session style updated.")
				continue
			}

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
	protocolText, err := readModelField(reader, "protocol (openai-chat-completions/anthropic-messages/google-generative-ai/mistral-chat/ollama-chat)", string(domainmodel.ProtocolOpenAIChatCompletions), true)
	if err != nil {
		return err
	}
	protocol := domainmodel.NormalizeProtocol(domainmodel.Protocol(protocolText))
	defaults, err := modelProtocolDefaults(protocol)
	if err != nil {
		return err
	}
	provider, err := readModelField(reader, "provider", defaults.provider, false)
	if err != nil {
		return err
	}
	authTypeText, err := readModelField(reader, "auth type (bearer/none)", string(defaults.authType), false)
	if err != nil {
		return err
	}
	authType := domainmodel.AuthType(strings.ToLower(strings.TrimSpace(authTypeText)))
	if authType != domainmodel.AuthTypeBearer && authType != domainmodel.AuthTypeNone {
		return fmt.Errorf("unsupported auth type: %s", authTypeText)
	}
	baseURL, err := readModelField(reader, "base url", defaults.baseURL, defaults.baseURLRequired)
	if err != nil {
		return err
	}
	apiKey, err := readModelField(reader, "api key (visible)", "", authType == domainmodel.AuthTypeBearer)
	if err != nil {
		return err
	}
	modelName, err := readModelField(reader, "model name", id, false)
	if err != nil {
		return err
	}
	temperatureText, err := readModelField(reader, "default temperature", "default", false)
	if err != nil {
		return err
	}
	topPText, err := readModelField(reader, "default top_p", "default", false)
	if err != nil {
		return err
	}
	maxTokensText, err := readModelField(reader, "default max output tokens", "default", false)
	if err != nil {
		return err
	}
	temperature, err := parseOptionalFloat(temperatureText, "temperature")
	if err != nil {
		return err
	}
	topP, err := parseOptionalFloat(topPText, "top_p")
	if err != nil {
		return err
	}
	maxOutputTokens, err := parseOptionalInt(maxTokensText, "max output tokens")
	if err != nil {
		return err
	}

	config := modelcommand.AddConfig{
		ID:        id,
		Name:      name,
		Provider:  provider,
		Protocol:  protocol,
		AuthType:  authType,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		ModelName: modelName,
		DefaultGenerationSettings: generation.Settings{
			Temperature:     temperature,
			TopP:            topP,
			MaxOutputTokens: maxOutputTokens,
		},
	}
	if err := chatService.AddModelConfig(ctx, config); err != nil {
		return err
	}

	printSuccess("model added: " + id)
	return nil
}

type modelProtocolInputDefaults struct {
	provider        string
	authType        domainmodel.AuthType
	baseURL         string
	baseURLRequired bool
}

func modelProtocolDefaults(protocol domainmodel.Protocol) (modelProtocolInputDefaults, error) {
	switch domainmodel.NormalizeProtocol(protocol) {
	case domainmodel.ProtocolOpenAIChatCompletions:
		return modelProtocolInputDefaults{provider: "openai", authType: domainmodel.AuthTypeBearer, baseURL: "https://api.openai.com/v1", baseURLRequired: true}, nil
	case domainmodel.ProtocolAnthropicMessages:
		return modelProtocolInputDefaults{provider: "anthropic", authType: domainmodel.AuthTypeBearer}, nil
	case domainmodel.ProtocolGoogleGenerativeAI:
		return modelProtocolInputDefaults{provider: "google", authType: domainmodel.AuthTypeBearer}, nil
	case domainmodel.ProtocolMistralChat:
		return modelProtocolInputDefaults{provider: "mistral", authType: domainmodel.AuthTypeBearer}, nil
	case domainmodel.ProtocolOllamaChat:
		return modelProtocolInputDefaults{provider: "ollama", authType: domainmodel.AuthTypeNone, baseURL: "http://127.0.0.1:11434", baseURLRequired: true}, nil
	default:
		return modelProtocolInputDefaults{}, fmt.Errorf("unsupported model protocol: %s", protocol)
	}
}

func parseGenerationSettings(input string) (generation.Settings, error) {
	fields := strings.Fields(input)
	if len(fields) != 5 || fields[0] != "/generation" || fields[1] != "set" {
		return generation.Settings{}, errors.New("usage: /generation set <temperature|default> <top_p|default> <max_tokens|default>")
	}
	temperature, err := parseOptionalFloat(fields[2], "temperature")
	if err != nil {
		return generation.Settings{}, err
	}
	topP, err := parseOptionalFloat(fields[3], "top_p")
	if err != nil {
		return generation.Settings{}, err
	}
	maxOutputTokens, err := parseOptionalInt(fields[4], "max tokens")
	if err != nil {
		return generation.Settings{}, err
	}
	return generation.Settings{
		Temperature:     temperature,
		TopP:            topP,
		MaxOutputTokens: maxOutputTokens,
	}, nil
}

func parseOptionalFloat(input string, name string) (*float64, error) {
	if strings.EqualFold(strings.TrimSpace(input), "default") {
		return nil, nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	return &value, nil
}

func parseOptionalInt(input string, name string) (*int, error) {
	if strings.EqualFold(strings.TrimSpace(input), "default") {
		return nil, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	return &value, nil
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
