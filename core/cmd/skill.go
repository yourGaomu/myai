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
	skillHubRegistry   string
	skillInstallForce  bool
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
		target, candidates, err := client.ResolveSkill(ctx, args[0])
		if err != nil {
			if ambiguous, ok := err.(*skillhub.AmbiguousSkillError); ok {
				printSkillHubCandidates(ambiguous.Candidates)
			}
			return err
		}
		if target.Slug != strings.TrimSpace(args[0]) || target.Namespace != "" {
			fmt.Printf("Installing matched skill: %s/%s\n", target.Namespace, target.Slug)
		}

		result, err := client.InstallSkill(ctx, skillhub.InstallRequest{Name: target.Slug, Namespace: target.Namespace, Force: skillInstallForce})
		if err != nil {
			return err
		}

		fmt.Println("Skill installed.")
		printSkillHubResult(result)
		if len(candidates) > 1 {
			fmt.Printf("Resolved from %d SkillHub search result(s).\n", len(candidates))
		}
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

	skillInstallCmd.Flags().BoolVar(&skillInstallForce, "force", false, "overwrite an already installed skill")

	skillCmd.PersistentFlags().StringVar(&skillWorkspace, "workspace", ".", "workspace directory")
	skillCmd.PersistentFlags().StringVar(&skillRoot, "root", skillhub.DefaultSkillDir, "local skill root directory")
	skillCmd.PersistentFlags().StringVar(&skillHubCommand, "skillhub", skillhub.DefaultCommand, "SkillHub CLI command")
	skillCmd.PersistentFlags().StringVar(&skillHubRegistry, "registry", "", "SkillHub registry URL")
	skillCmd.PersistentFlags().DurationVar(&skillCommandTimout, "timeout", 2*time.Minute, "SkillHub command timeout")
}

func newSkillHubClient() *skillhub.Client {
	return skillhub.NewClient(skillhub.Options{
		Workspace: skillWorkspace,
		SkillRoot: skillRoot,
		Command:   skillHubCommand,
		Registry:  skillHubRegistry,
	})
}

func printSkillHubResult(result skillhub.CommandResult) {
	fmt.Println("Workdir:", result.WorkDir)
	fmt.Println("Command:", result.Command)
	if result.Output != "" {
		fmt.Println(result.Output)
	}
}

func printSkillHubCandidates(items []skillhub.SearchItem) {
	if len(items) == 0 {
		return
	}
	fmt.Println("Matched SkillHub skills:")
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		item := items[i]
		name := item.Slug
		if item.Namespace != "" {
			name = item.Namespace + "/" + item.Slug
		}
		line := "- " + name
		if item.Summary != "" {
			line += ": " + item.Summary
		}
		fmt.Println(line)
	}
	if len(items) > limit {
		fmt.Printf("...and %d more. Use `myai skill search <query>` to inspect them.\n", len(items)-limit)
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
