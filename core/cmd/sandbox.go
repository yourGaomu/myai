package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	opensandboxadapter "myai/core/adapter/sandbox/opensandbox"
	appconfig "myai/core/config"
	domainsandbox "myai/core/domain/sandbox"
)

var (
	sandboxWorkspace string
	sandboxCommand   string
	sandboxTimeout   time.Duration
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage isolated sandbox environments",
}

var sandboxDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Create, execute, and destroy a temporary OpenSandbox instance",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), sandboxTimeout)
		defer cancel()

		properties, err := (appconfig.ViperLoader{}).Load(sandboxWorkspace)
		if err != nil {
			return fmt.Errorf("load application config: %w", err)
		}
		if strings.ToLower(strings.TrimSpace(properties.Sandbox.Provider)) != "opensandbox" {
			return fmt.Errorf("sandbox.provider must be opensandbox, got %q", properties.Sandbox.Provider)
		}
		manager, err := opensandboxadapter.New((appconfig.Mapper{}).OpenSandboxConfig(properties.Sandbox.OpenSandbox))
		if err != nil {
			return err
		}

		workspace, err := manager.Create(ctx, domainsandbox.CreateRequest{
			NetworkPolicy: &domainsandbox.NetworkPolicy{DefaultAction: domainsandbox.NetworkActionDeny},
		})
		if err != nil {
			return fmt.Errorf("create OpenSandbox instance: %w", err)
		}
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cleanupCancel()
			_ = workspace.Destroy(cleanupCtx)
			_ = workspace.Close()
		}()

		fmt.Println("Sandbox:", workspace.ID())
		result, err := workspace.Run(ctx, domainsandbox.CommandRequest{Command: sandboxCommand})
		if err != nil {
			return err
		}
		fmt.Println("Exit code:", result.ExitCode)
		if result.Stdout != "" {
			fmt.Println("Stdout:")
			fmt.Println(result.Stdout)
		}
		if result.Stderr != "" {
			fmt.Println("Stderr:")
			fmt.Println(result.Stderr)
		}
		if result.TimedOut {
			return fmt.Errorf("sandbox command timed out")
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("sandbox command exited with code %d", result.ExitCode)
		}
		fmt.Println("OpenSandbox doctor passed.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sandboxCmd)
	sandboxCmd.AddCommand(sandboxDoctorCmd)

	sandboxCmd.PersistentFlags().StringVar(&sandboxWorkspace, "workspace", ".", "workspace directory containing application.yaml")
	sandboxDoctorCmd.Flags().StringVar(&sandboxCommand, "command", "python -c \"print('OpenSandbox OK')\"", "command to execute inside the temporary sandbox")
	sandboxDoctorCmd.Flags().DurationVar(&sandboxTimeout, "timeout", 2*time.Minute, "overall doctor timeout")
}
