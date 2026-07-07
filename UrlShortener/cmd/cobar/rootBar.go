package cobar

import (
	"context"
	"fmt"
	"log"
	"time"

	"myai-url-shortener/internal/shortener/handle"
	"myai-url-shortener/internal/shortener/service"
	"myai-url-shortener/internal/shortener/urlConfig"

	"github.com/spf13/cobra"
)

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:          "url-shortener",
		Short:        "Start the MyAI URL shortener service",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), configPath)
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", "", "path to resource/application.yaml config file")
	return rootCmd
}

func run(parent context.Context, configPath string) error {
	config, err := urlConfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	linkStore, cleanup, err := newLinkStore(ctx, config)
	if err != nil {
		return fmt.Errorf("create store failed: %w", err)
	}
	defer cleanup()

	objectStore, err := newObjectStore(ctx, config)
	if err != nil {
		return fmt.Errorf("create object store failed: %w", err)
	}

	newService := service.NewService(linkStore, service.ServiceOptions{
		BaseURL:      config.BaseURL,
		DefaultTTL:   config.DefaultTTL,
		ObjectStore:  objectStore,
		ObjectURLTTL: config.ObjectURLTTL,
	})

	router := handle.NewRouter(newService)

	log.Printf("url shortener listening on %s", config.Addr)
	if err := router.Run(config.Addr); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
