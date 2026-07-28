package opensandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/alibaba/OpenSandbox/sdks/sandbox/go"

	domainsandbox "myai/core/domain/sandbox"
	sandboxport "myai/core/port/sandbox"
)

type Manager struct {
	config     Config
	connection sdk.ConnectionConfig
	admin      *sdk.SandboxManager
	create     createSandboxFunc
	connect    connectSandboxFunc
}

var _ sandboxport.Manager = (*Manager)(nil)

func New(config Config) (*Manager, error) {
	config = config.normalize()
	if err := config.validate(); err != nil {
		return nil, err
	}
	connection, err := config.connectionConfig()
	if err != nil {
		return nil, err
	}
	return &Manager{
		config:     config,
		connection: connection,
		admin:      sdk.NewSandboxManager(connection),
		create:     createSDKInstance,
		connect:    connectSDKInstance,
	}, nil
}

func (manager *Manager) Create(ctx context.Context, request domainsandbox.CreateRequest) (sandboxport.Workspace, error) {
	if manager == nil || manager.create == nil {
		return nil, fmt.Errorf("OpenSandbox manager is not initialized")
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("validate OpenSandbox create request: %w", err)
	}
	options := manager.createOptions(request)
	instance, err := manager.create(ctx, manager.connection, options)
	if err != nil {
		return nil, err
	}
	return &Workspace{client: instance, commandTimeout: manager.config.CommandTimeout, maxDownloadBytes: manager.config.MaxDownloadBytes}, nil
}

func (manager *Manager) Connect(ctx context.Context, sandboxID string) (sandboxport.Workspace, error) {
	if manager == nil || manager.connect == nil {
		return nil, fmt.Errorf("OpenSandbox manager is not initialized")
	}
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, fmt.Errorf("OpenSandbox ID is required")
	}
	instance, err := manager.connect(ctx, manager.connection, sandboxID)
	if err != nil {
		return nil, err
	}
	return &Workspace{client: instance, commandTimeout: manager.config.CommandTimeout, maxDownloadBytes: manager.config.MaxDownloadBytes}, nil
}

func (manager *Manager) Delete(ctx context.Context, sandboxID string) error {
	if manager == nil || manager.admin == nil {
		return fmt.Errorf("OpenSandbox manager is not initialized")
	}
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return fmt.Errorf("OpenSandbox ID is required")
	}
	return manager.admin.KillSandbox(ctx, sandboxID)
}

func (manager *Manager) createOptions(request domainsandbox.CreateRequest) sdk.SandboxCreateOptions {
	image := strings.TrimSpace(request.Image)
	if image == "" {
		image = manager.config.Image
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = manager.config.SandboxTimeout
	}
	timeoutSeconds := int((timeout + time.Second - 1) / time.Second)
	cpu := strings.TrimSpace(request.CPU)
	if cpu == "" {
		cpu = manager.config.CPU
	}
	memory := strings.TrimSpace(request.Memory)
	if memory == "" {
		memory = manager.config.Memory
	}
	return sdk.SandboxCreateOptions{
		Image:          image,
		Entrypoint:     append([]string(nil), request.Entrypoint...),
		ResourceLimits: sdk.ResourceLimits{"cpu": cpu, "memory": memory},
		TimeoutSeconds: &timeoutSeconds,
		Env:            cloneMap(request.Env),
		Metadata:       cloneMap(request.Metadata),
		NetworkPolicy:  mapNetworkPolicy(request.NetworkPolicy),
	}
}

func mapNetworkPolicy(policy *domainsandbox.NetworkPolicy) *sdk.NetworkPolicy {
	if policy == nil {
		return nil
	}
	rules := make([]sdk.NetworkRule, 0, len(policy.Rules))
	for _, rule := range policy.Rules {
		rules = append(rules, sdk.NetworkRule{Action: string(rule.Action), Target: strings.TrimSpace(rule.Target)})
	}
	return &sdk.NetworkPolicy{DefaultAction: string(policy.DefaultAction), Egress: rules}
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
