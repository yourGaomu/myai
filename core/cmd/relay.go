package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/mongo"

	memoryauthorization "myai/core/adapter/authorization/memory"
	mongoauthorization "myai/core/adapter/persistence/mongo/authorization"
	appconfig "myai/core/config"
	"myai/core/infra"
	"myai/core/remote/relay"
)

var (
	relayAddr       string
	relayConfigFile string
)

var relayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Start myai relay server",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Relay 默认使用内存授权；配置 Mongo 后替换为持久化授权仓库。
		server := relay.NewServer(relayAddr, memoryauthorization.NewStore())
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
