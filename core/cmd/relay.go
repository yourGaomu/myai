package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/mongo"

	memoryauthorization "myai/core/adapter/authorization/memory"
	mongoauthorization "myai/core/adapter/persistence/mongo/authorization/repository"
	appconfig "myai/core/config"
	"myai/core/infra"
	"myai/core/remote/relay"
)

var (
	relayAddr        string
	relayConfigFile  string
	relayAgentToken  string
	relayAgentUser   string
	relayAgentDevice string
	relayCredentials []string
	relayOrigins     []string
)

var relayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Start myai relay server",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		agentCredentials, err := configuredRelayAgentCredentials()
		if err != nil {
			return err
		}

		// Relay 默认使用内存授权；配置 Mongo 后替换为持久化授权仓库。
		server := relay.NewServer(
			relayAddr,
			memoryauthorization.NewStore(),
			relay.WithAgentCredentials(agentCredentials...),
			relay.WithAllowedOrigins(relayOrigins...),
		)
		mongoClient, err := configureRelayAuthStore(ctx, server)
		if err != nil {
			return err
		}
		if mongoClient != nil {
			defer func() {
				disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = mongoClient.Disconnect(disconnectCtx)
			}()
		}

		return server.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(relayCmd)

	relayCmd.Flags().StringVar(&relayAddr, "addr", ":8080", "relay server listen address")
	relayCmd.Flags().StringVar(&relayConfigFile, "urlConfig", "./resource/application.yaml", "urlConfig file for relay persistence")
	relayCmd.Flags().StringVar(&relayAgentToken, "agent-token", os.Getenv("MYAI_RELAY_AGENT_TOKEN"), "token for the primary relay agent identity")
	relayCmd.Flags().StringVar(&relayAgentUser, "agent-user", environmentOrDefault("MYAI_RELAY_AGENT_USER", "local"), "user id bound to --agent-token")
	relayCmd.Flags().StringVar(&relayAgentDevice, "agent-device", environmentOrDefault("MYAI_RELAY_AGENT_DEVICE", "pc-local"), "device id bound to --agent-token")
	relayCmd.Flags().StringSliceVar(&relayCredentials, "agent-credential", splitRelayCredentialList(os.Getenv("MYAI_RELAY_AGENT_CREDENTIALS")), "additional identity-bound credential in user/device=token format")
	relayCmd.Flags().StringSliceVar(&relayOrigins, "allowed-origin", nil, "additional browser origins allowed by CORS and WebSocket checks")
}

func configuredRelayAgentCredentials() ([]relay.AgentCredential, error) {
	values := make([]relay.AgentCredential, 0, len(relayCredentials)+1)
	seen := make(map[string]struct{})
	add := func(userID string, deviceID string, token string) error {
		userID = strings.TrimSpace(userID)
		deviceID = strings.TrimSpace(deviceID)
		token = strings.TrimSpace(token)
		if userID == "" || deviceID == "" || token == "" {
			return errors.New("relay agent credential requires non-empty user, device, and token")
		}
		key := userID + "/" + deviceID
		if _, exists := seen[key]; exists {
			return errors.New("duplicate relay agent credential: " + key)
		}
		seen[key] = struct{}{}
		values = append(values, relay.AgentCredential{UserID: userID, DeviceID: deviceID, Token: token})
		return nil
	}

	if strings.TrimSpace(relayAgentToken) != "" {
		if err := add(relayAgentUser, relayAgentDevice, relayAgentToken); err != nil {
			return nil, err
		}
	}
	for _, raw := range relayCredentials {
		identity, token, found := strings.Cut(strings.TrimSpace(raw), "=")
		if !found {
			return nil, errors.New("invalid relay agent credential, expected user/device=token")
		}
		userID, deviceID, found := strings.Cut(identity, "/")
		if !found {
			return nil, errors.New("invalid relay agent identity, expected user/device")
		}
		if err := add(userID, deviceID, token); err != nil {
			return nil, err
		}
	}
	if len(values) == 0 {
		return nil, errors.New("relay agent credential is required; configure --agent-token or --agent-credential")
	}
	return values, nil
}

func splitRelayCredentialList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' })
}

func environmentOrDefault(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func configureRelayAuthStore(ctx context.Context, server *relay.Server) (*mongo.Client, error) {
	properties, found, err := (appconfig.ViperLoader{ConfigFile: relayConfigFile}).LoadOptional("")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	uri := properties.Mongo.URI
	database := properties.Mongo.Database
	if uri == "" || database == "" {
		return nil, nil
	}

	mongoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := infra.NewMongoClient(mongoCtx, uri)
	if err != nil {
		return nil, err
	}

	server.SetAuthStore(mongoauthorization.New(client, database))
	return client, nil
}
