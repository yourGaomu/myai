package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"myai/core/skill"
	"myai/core/skillhub"
)

var (
	skillWorkspace     string
	skillRoot          string
	skillHubCommand    string
	skillCommandTimout time.Duration
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage local skills",
}

var skillInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a SkillHub skill into the current workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), skillCommandTimout)
		defer cancel()

		client := newSkillHubClient()
		result, err := client.InstallSkill(ctx, skillhub.InstallRequest{Name: args[0]})
		if err != nil {
			return err
		}

		fmt.Println("Skill installed.")
		printSkillHubResult(result)
		return printReloadedSkills(ctx, client)
	},
}

var skillSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search SkillHub skills",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), skillCommandTimout)
		defer cancel()

		result, err := newSkillHubClient().Search(ctx, skillhub.SearchRequest{Query: args[0]})
		if err != nil {
			return err
		}
		printSkillHubResult(result)
		return nil
	},
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List local skills loaded from SKILL.md files",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), skillCommandTimout)
		defer cancel()

		return printLocalSkills(ctx, newSkillHubClient(), "")
	},
}

var skillReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload and list local skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), skillCommandTimout)
		defer cancel()

		return printLocalSkills(ctx, newSkillHubClient(), "Skills reloaded.")
	},
}

var skillInstallCLICmd = &cobra.Command{
	Use:   "install-cli",
	Short: "Install only the SkillHub CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), skillCommandTimout)
		defer cancel()

		result, err := newSkillHubClient().InstallCLI(ctx)
		if err != nil {
			return err
		}
		fmt.Println("SkillHub CLI installed.")
		printSkillHubResult(result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(skillCmd)
	skillCmd.AddCommand(skillInstallCmd, skillSearchCmd, skillListCmd, skillReloadCmd, skillInstallCLICmd)

	skillCmd.PersistentFlags().StringVar(&skillWorkspace, "workspace", ".", "workspace directory")
	skillCmd.PersistentFlags().StringVar(&skillRoot, "root", skillhub.DefaultSkillDir, "local skill root directory")
	skillCmd.PersistentFlags().StringVar(&skillHubCommand, "skillhub", skillhub.DefaultCommand, "SkillHub CLI command")
	skillCmd.PersistentFlags().DurationVar(&skillCommandTimout, "timeout", 2*time.Minute, "SkillHub command timeout")
}

func newSkillHubClient() *skillhub.Client {
	return skillhub.NewClient(skillhub.Options{
		Workspace: skillWorkspace,
		SkillRoot: skillRoot,
		Command:   skillHubCommand,
	})
}

func printSkillHubResult(result skillhub.CommandResult) {
	fmt.Println("Workdir:", result.WorkDir)
	fmt.Println("Command:", result.Command)
	if result.Output != "" {
		fmt.Println(result.Output)
	}
}

func printReloadedSkills(ctx context.Context, client *skillhub.Client) error {
	return printLocalSkills(ctx, client, "")
}

func printLocalSkills(ctx context.Context, client *skillhub.Client, message string) error {
	root, err := client.SkillRootPath()
	if err != nil {
		return err
	}

	manager := skill.NewManager(root)
	if err := manager.Reload(ctx); err != nil {
		return err
	}
	if message != "" {
		fmt.Println(message)
	}
	skills := manager.List()
	if len(skills) == 0 {
		fmt.Println("No SKILL.md files found under:", root)
		fmt.Println("If SkillHub installed somewhere else, pass --root or configure skill.root to that directory.")
		return nil
	}

	fmt.Printf("Loaded %d local skill(s) from %s.\n", len(skills), root)
	for _, item := range skills {
		line := fmt.Sprintf("- %s", item.Name)
		if item.Description != "" {
			line += ": " + item.Description
		}
		if len(item.Triggers) > 0 {
			line += " (triggers: " + strings.Join(item.Triggers, ", ") + ")"
		}
		fmt.Println(line)
	}
	return nil
}
