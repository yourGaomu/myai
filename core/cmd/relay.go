package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/v2/mongo"

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

		server := relay.NewServer(relayAddr)
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
	relayCmd.Flags().StringVar(&relayConfigFile, "config", "./resource/application.yaml", "config file for relay persistence")
}

func configureRelayAuthStore(ctx context.Context, server *relay.Server) (*mongo.Client, error) {
	config := viper.New()
	config.SetConfigFile(relayConfigFile)
	if err := config.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	uri := config.GetString("mongo.uri")
	database := config.GetString("mongo.database")
	if uri == "" || database == "" {
		return nil, nil
	}

	mongoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := infra.NewMongoClient(mongoCtx, uri)
	if err != nil {
		return nil, err
	}

	server.SetAuthStore(relay.NewMongoAuthStore(client, database))
	return client, nil
}
