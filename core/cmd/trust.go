package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	appconfig "myai/core/config"
	"myai/core/hook"
	"myai/core/mcp"
	"myai/core/plugin"
	"myai/core/trust"
)

func init() { rootCmd.AddCommand(newTrustCommand(nil)) }

type trustOptions struct {
	workspace  string
	config     string
	kind       string
	id         string
	pluginRoot string
}

// newTrustCommand uses the user-scoped store. Tests inject a separate store;
// workspace configuration can never select the store or assert its own trust.
func newTrustCommand(store *trust.Store) *cobra.Command {
	options := &trustOptions{}
	command := &cobra.Command{
		Use:   "trust",
		Short: "Review and approve executable workspace declarations",
		Long:  "List executable declarations without running them. Review the local configuration, then approve its full digest. Changed declarations require a new approval. Approvals are stored in your user configuration directory.",
	}
	flags := command.PersistentFlags()
	flags.StringVar(&options.workspace, "workspace", ".", "workspace directory")
	flags.StringVar(&options.config, "config", "", "declaration config file (MCP: relative to the current directory, default ./resource/application.yaml; hooks/plugins: relative to workspace)")
	flags.StringVar(&options.pluginRoot, "root", "", "plugin root override, relative to workspace")
	flags.StringVar(&options.kind, "kind", "", "declaration kind: hook, plugin, or mcp")
	flags.StringVar(&options.id, "id", "", "complete declaration digest from trust list")
	addActions := func(parent *cobra.Command, fixedKind trust.Kind) {
		for _, action := range []string{"list", "approve", "revoke"} {
			action := action
			parent.AddCommand(&cobra.Command{
				Use:   action,
				Short: action + " user approval for a specific declaration",
				Args:  cobra.NoArgs,
				RunE: func(cmd *cobra.Command, _ []string) error {
					kind := trust.Kind(strings.ToLower(strings.TrimSpace(options.kind)))
					if fixedKind != "" {
						if kind != "" && kind != fixedKind {
							return errors.New("--kind does not match the subcommand")
						}
						kind = fixedKind
					}
					if kind != trust.KindHook && kind != trust.KindPlugin && kind != trust.KindMCP {
						return errors.New("select --kind hook, plugin, or mcp")
					}
					return runTrustAction(cmd, store, *options, kind, action)
				},
			})
		}
	}
	addActions(command, "")
	for _, kind := range []trust.Kind{trust.KindHook, trust.KindPlugin, trust.KindMCP} {
		branch := &cobra.Command{Use: string(kind), Short: "Review " + string(kind) + " executable declarations"}
		addActions(branch, kind)
		command.AddCommand(branch)
	}
	return command
}

func runTrustAction(command *cobra.Command, store *trust.Store, options trustOptions, kind trust.Kind, action string) error {
	if err := command.Context().Err(); err != nil {
		return err
	}
	workspace, err := filepath.Abs(options.workspace)
	if err != nil {
		return errors.New("resolve workspace directory")
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return errors.New("workspace directory does not exist")
	}
	if store == nil {
		store, err = trust.DefaultStore()
		if err != nil {
			return err
		}
	}
	stored, err := store.List(workspace, kind)
	if err != nil {
		return err
	}
	if action == "revoke" {
		id := strings.ToLower(strings.TrimSpace(options.id))
		if len(id) != 64 {
			return errors.New("--id must be the complete SHA256 digest from trust list")
		}
		for _, request := range stored {
			if request.Digest == id {
				if err := store.Revoke(request); err != nil {
					return err
				}
				fmt.Fprintf(command.OutOrStdout(), "revoke %s %s\n", kind, id)
				return nil
			}
		}
		return errors.New("no matching user approval; use trust list to view stored IDs")
	}
	requests, err := readTrustRequests(workspace, options.config, options.pluginRoot, kind)
	if err != nil {
		return err
	}
	if action == "list" {
		writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "KIND\tSTATUS\tID (SHA256)\tDECLARATION\tPATH")
		currentIDs := make(map[string]bool, len(requests))
		for _, request := range requests {
			currentIDs[request.Digest] = true
			approved, err := store.Trusted(request)
			if err != nil {
				return err
			}
			status := "untrusted"
			if approved {
				status = "approved"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%q\n", request.Kind, status, request.Digest, trustSummary(request), request.Path)
		}
		for _, request := range stored {
			if !currentIDs[request.Digest] {
				fmt.Fprintf(writer, "%s\tapproved-stale\t%s\tdeclaration no longer present\t%q\n", request.Kind, request.Digest, request.Path)
			}
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		if kind == trust.KindMCP {
			fmt.Fprintln(command.OutOrStdout(), "Review HTTP endpoints and header declarations or stdio commands, arguments, and environment in the Agent configuration before approving the full ID. Values are hidden here.")
		} else {
			fmt.Fprintln(command.OutOrStdout(), "Review command, arguments, and environment in the local declaration file before approving its full ID. Values are hidden here.")
		}
		return nil
	}
	id := strings.ToLower(strings.TrimSpace(options.id))
	if len(id) != 64 {
		return errors.New("--id must be the complete SHA256 digest from trust list")
	}
	for _, request := range requests {
		if request.Digest != id {
			continue
		}
		if err := store.Approve(request); err != nil {
			return err
		}
		fmt.Fprintf(command.OutOrStdout(), "%s %s %s\n", action, kind, id)
		return nil
	}
	return errors.New("declaration ID is no longer present; list and review the current configuration")
}

// Reading the workspace configuration is limited to an explicit user trust
// command. Errors never echo YAML source, command arguments, or credential data.
func readTrustRequests(workspace, configPath, pluginRoot string, kind trust.Kind) ([]trust.Request, error) {
	if kind == trust.KindMCP {
		properties, err := (appconfig.ViperLoader{ConfigFile: configPath}).LoadMCP()
		if err != nil {
			return nil, err
		}
		config := (appconfig.Mapper{}).MCPConfig(workspace, properties)
		var requests []trust.Request
		for _, server := range config.Servers {
			if server.Disabled {
				continue
			}
			request, err := mcp.TrustRequest(server, workspace)
			if err != nil {
				return nil, err
			}
			requests = append(requests, request)
		}
		return requests, nil
	}
	if configPath == "" {
		configPath = filepath.Join(workspace, "resource", "application.yaml")
	} else if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(workspace, configPath)
	}
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return nil, errors.New("cannot read declaration configuration")
	}
	switch kind {
	case trust.KindHook:
		var properties appconfig.HookProperties
		if err := v.UnmarshalKey("hooks.commands", &properties.Commands); err != nil {
			return nil, errors.New("invalid hook declarations")
		}
		requests, err := hook.ReviewRequests((appconfig.Mapper{}).HookConfig(workspace, properties))
		if err != nil {
			return nil, errors.New("invalid hook declaration; check event, timeout, command, and work_dir locally")
		}
		return requests, nil
	case trust.KindPlugin:
		if pluginRoot == "" {
			pluginRoot = strings.TrimSpace(v.GetString("plugin.root"))
		}
		if pluginRoot == "" {
			pluginRoot = "plugins"
		}
		if !filepath.IsAbs(pluginRoot) {
			pluginRoot = filepath.Join(workspace, pluginRoot)
		}
		return plugin.ReviewRequests(pluginRoot, workspace)
	}
	return nil, errors.New("invalid declaration kind")
}

func trustSummary(request trust.Request) string {
	var fields struct {
		Event    string            `json:"event"`
		Name     string            `json:"name"`
		URL      string            `json:"url"`
		Headers  map[string]string `json:"headers"`
		Command  string            `json:"command"`
		Args     []string          `json:"args"`
		Env      map[string]string `json:"env"`
		Manifest plugin.Manifest   `json:"manifest"`
	}
	if err := json.Unmarshal(request.Declaration, &fields); err != nil {
		return "executable declaration"
	}
	switch request.Kind {
	case trust.KindHook:
		shell := "sh"
		if runtime.GOOS == "windows" {
			shell = "powershell"
		}
		return fmt.Sprintf("event=%q shell=%s fields=command,timeout,work_dir", fields.Event, shell)
	case trust.KindPlugin:
		return fmt.Sprintf("plugin=%q executable=%q args=%d env=%d", fields.Manifest.ID, filepath.Base(fields.Command), len(fields.Manifest.Args), len(fields.Manifest.Env))
	default:
		if fields.URL != "" {
			return fmt.Sprintf("server=%q transport=http headers=%d", fields.Name, len(fields.Headers))
		}
		return fmt.Sprintf("server=%q executable=%q args=%d env=%d", fields.Name, filepath.Base(fields.Command), len(fields.Args), len(fields.Env))
	}
}
