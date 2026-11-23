package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"

	modelcommand "myai/core/application/model/command"
	"myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
	"myai/core/mcp"
	pluginruntime "myai/core/plugin"
	"myai/core/remote/protocol"
	"myai/core/skill"
)

// Lifecycle operations may wait on external processes. Never run them on the
// websocket reader: permission replies and pause requests must remain readable.
func (a *Agent) enqueueCatalogMutation(ctx context.Context, conn *websocket.Conn, message protocol.Message, handle func(context.Context, *websocket.Conn, protocol.Message) error) {
	if err := a.catalogOperations.enqueue(ctx, func() {
		if err := handle(ctx, conn, message); err != nil {
			if writeErr := a.writeHandlerError(conn, message, err); writeErr != nil {
				log.Printf("send catalog mutation error: %v", writeErr)
			}
		}
	}); err != nil {
		if writeErr := a.writeHandlerError(conn, message, err); writeErr != nil {
			log.Printf("send catalog queue error: %v", writeErr)
		}
	}
}

func (a *Agent) handleModelList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload := a.modelListPayload()
	return a.writeRemoteMessage(conn, protocol.TypeModelListResult, message.RequestID, message.SessionID, payload)
}

func (a *Agent) handleSkillList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if _, err := protocol.DecodePayload[protocol.SkillListPayload](message); err != nil {
		return fmt.Errorf("decode skill list failed: %w", err)
	}

	payload, err := a.skillListPayload(ctx, false)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSkillListResult, message.RequestID, message.SessionID, payload)
}

func (a *Agent) handleSkillReload(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if _, err := protocol.DecodePayload[protocol.SkillReloadPayload](message); err != nil {
		return fmt.Errorf("decode skill reload failed: %w", err)
	}

	payload, err := a.skillListPayload(ctx, true)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeSkillReloadResult, message.RequestID, message.SessionID, payload)
}

func (a *Agent) handlePluginList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if _, err := protocol.DecodePayload[protocol.PluginListPayload](message); err != nil {
		return fmt.Errorf("decode plugin list failed: %w", err)
	}
	if a.pluginManager == nil {
		return fmt.Errorf("plugin manager is not configured")
	}
	return a.writeRemoteMessage(conn, protocol.TypePluginListResult, message.RequestID, message.SessionID, a.pluginListPayload(false, ""))
}

func (a *Agent) handlePluginReload(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if _, err := protocol.DecodePayload[protocol.PluginListPayload](message); err != nil {
		return fmt.Errorf("decode plugin reload failed: %w", err)
	}
	if a.pluginManager == nil {
		return fmt.Errorf("plugin manager is not configured")
	}
	a.catalogMu.Lock()
	defer a.catalogMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.pluginManager.Reload(ctx); err != nil {
		return err
	}
	payload := a.pluginListPayload(true, "插件已重载。")
	return a.writeRemoteMessage(conn, protocol.TypePluginReloadResult, message.RequestID, message.SessionID, payload)
}

func (a *Agent) handleMCPList(conn *websocket.Conn, message protocol.Message) error {
	if _, err := protocol.DecodePayload[protocol.MCPListPayload](message); err != nil {
		return fmt.Errorf("invalid mcp list request")
	}
	var statuses []mcp.ServerStatus
	var revision *uint64
	var err error
	switch reader := a.mcpManager.(type) {
	case mcpCatalogStatusReader:
		var version uint64
		statuses, version, err = reader.MCPCatalogStatuses()
		revision = &version
	case mcpStatusReader:
		statuses, err = reader.MCPServerStatuses()
	default:
		return errors.New("mcp inventory is unavailable")
	}
	if err != nil {
		return errors.New("mcp inventory is unavailable")
	}
	servers := make([]protocol.MCPServerSummary, 0, len(statuses))
	for _, status := range statuses {
		servers = append(servers, mcpServerSummary(status))
	}
	return a.writeRemoteMessage(conn, protocol.TypeMCPListResult, message.RequestID, message.SessionID, protocol.MCPListResultPayload{
		Servers: servers, CatalogRevision: revision,
	})
}

func mcpServerSummary(status mcp.ServerStatus) protocol.MCPServerSummary {
	result := protocol.MCPServerSummary{
		Name: status.Name, State: string(status.State),
		InventoryToolCount: status.InventoryToolCount, ExposedToolCount: status.ExposedToolCount,
	}
	switch status.Reason {
	case "pending_trust", "start_failed", "discovery_failed":
		result.Reason = status.Reason
	}
	return result
}

func (a *Agent) handleMCPDetail(conn *websocket.Conn, message protocol.Message) error {
	request, err := protocol.MCPDetailRequest(message)
	if err != nil {
		return err
	}
	if _, err := a.uploadOwner(message); err != nil {
		return errors.New("mcp detail requires an authenticated paired client")
	}
	reader, ok := a.mcpManager.(mcpDetailReader)
	if !ok {
		return errors.New("mcp detail is not available on this Agent")
	}
	writeFailure := func(text, code string) error {
		return a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.MCPDetailErrorPayload{
			ErrorPayload: protocol.ErrorPayload{Message: text, Code: code},
			RequestType:  protocol.TypeMCPDetail, ServerName: request.ServerName,
		})
	}
	detail, err := reader.MCPServerDetail(request.ServerName, message.SessionID, request.Cursor)
	switch {
	case errors.Is(err, mcp.ErrCatalogChanged):
		return writeFailure("mcp catalog changed; refresh and retry", protocol.MCPErrorCatalogChanged)
	case errors.Is(err, mcp.ErrCatalogServerNotFound):
		return writeFailure("mcp server was not found", "")
	case errors.Is(err, mcp.ErrCatalogInvalidCursor):
		return writeFailure("invalid mcp catalog cursor", "")
	case err != nil, detail.Server.Name != request.ServerName, len(detail.Tools) > protocol.MCPDetailMaxTools:
		return writeFailure("mcp detail is unavailable", "")
	}
	tools := make([]protocol.MCPToolDescriptor, 0, len(detail.Tools))
	for _, tool := range detail.Tools {
		tools = append(tools, protocol.MCPToolDescriptor{
			ID: tool.ID, Name: tool.Name, Title: tool.Title, Description: tool.Description, Alias: tool.Alias,
			HostOnly: tool.HostOnly, MetadataTruncated: tool.MetadataTruncated, Input: mcpSchemaSummary(tool.Input),
			Output: mcpSchemaSummary(tool.Output), Eligibility: tool.Eligibility, Reason: tool.Reason,
		})
	}
	response, err := protocol.NewMessage(protocol.TypeMCPDetailResult, message.RequestID, a.config.UserID, a.config.DeviceID, message.SessionID, protocol.MCPDetailResultPayload{
		Server: mcpServerSummary(detail.Server), CatalogRevision: detail.CatalogRevision, SessionEvaluated: detail.SessionEvaluated,
		Tools: tools, NextCursor: detail.NextCursor,
	})
	if err != nil {
		return writeFailure("mcp detail is unavailable", "")
	}
	if _, err := protocol.MCPDetailResult(response); err != nil {
		return writeFailure("mcp detail is unavailable", "")
	}
	return a.writeJSON(conn, response)
}

func mcpSchemaSummary(summary mcp.SchemaSummary) protocol.MCPSchemaSummary {
	result := protocol.MCPSchemaSummary{State: summary.State, Truncated: summary.Truncated}
	for _, field := range summary.Fields {
		result.Fields = append(result.Fields, protocol.MCPSchemaField(field))
	}
	return result
}

func (a *Agent) handleMCPReload(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if _, err := protocol.DecodePayload[protocol.MCPReloadPayload](message); err != nil {
		return fmt.Errorf("decode mcp reload failed: %w", err)
	}
	if a.mcpManager == nil {
		return fmt.Errorf("mcp manager is not configured")
	}
	a.catalogMu.Lock()
	defer a.catalogMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.mcpManager.ReloadMCP(ctx); err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeMCPReloadResult, message.RequestID, message.SessionID, protocol.MCPReloadResultPayload{
		Reloaded: true,
		Message:  "MCP servers reloaded.",
	})
}

func (a *Agent) handlePluginToggle(ctx context.Context, conn *websocket.Conn, message protocol.Message, enabled bool) error {
	payload, err := protocol.DecodePayload[protocol.PluginTogglePayload](message)
	if err != nil {
		return fmt.Errorf("decode plugin toggle failed: %w", err)
	}
	if a.pluginManager == nil {
		return fmt.Errorf("plugin manager is not configured")
	}
	if strings.TrimSpace(payload.PluginID) == "" {
		return fmt.Errorf("plugin id is required")
	}
	a.catalogMu.Lock()
	defer a.catalogMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.pluginManager.SetEnabled(ctx, payload.PluginID, enabled); err != nil {
		return err
	}
	state := "已禁用"
	if enabled {
		state = "已启用"
	}
	return a.writeRemoteMessage(conn, protocol.TypePluginMutationResult, message.RequestID, message.SessionID, protocol.PluginMutationResultPayload{
		PluginID: payload.PluginID,
		Enabled:  enabled,
		Plugins:  mapPluginInfos(a.pluginManager.List()),
		Count:    len(a.pluginManager.List()),
		Message:  fmt.Sprintf("插件 %s %s。", payload.PluginID, state),
	})
}

func (a *Agent) pluginListPayload(reloaded bool, message string) protocol.PluginListResultPayload {
	if a.pluginManager == nil {
		return protocol.PluginListResultPayload{Message: "插件管理器未配置。"}
	}
	items := a.pluginManager.List()
	if message == "" && reloaded {
		message = fmt.Sprintf("已重载 %d 个插件。", len(items))
	}
	return protocol.PluginListResultPayload{
		Root:     a.pluginManager.Root(),
		Plugins:  mapPluginInfos(items),
		Count:    len(items),
		Reloaded: reloaded,
		Message:  message,
	}
}

func mapPluginInfos(items []pluginruntime.Info) []protocol.PluginInfo {
	result := make([]protocol.PluginInfo, 0, len(items))
	for _, item := range items {
		result = append(result, protocol.PluginInfo{
			ID: item.Manifest.ID, Name: item.Manifest.Name, Version: item.Manifest.Version,
			Protocol: item.Manifest.Protocol, Entrypoint: item.Manifest.Entrypoint,
			Directory: item.Directory, Status: string(item.Status), Error: item.Error,
			Enabled: item.Manifest.Enabled == nil || *item.Manifest.Enabled, Required: item.Manifest.Required,
		})
	}
	return result
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func (a *Agent) modelListPayload() protocol.ModelListResultPayload {
	return protocol.ModelListResultPayload{
		CurrentModelID: a.chatService.CurrentModelID(),
		Models:         modelSummaries(a.chatService.ListModels()),
	}
}

func (a *Agent) handleModelConfigAdd(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelConfigAddPayload](message)
	if err != nil {
		return fmt.Errorf("decode model config add failed: %w", err)
	}

	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := a.chatService.AddModelConfig(ctx, modelcommand.AddConfig{
		ID:                  payload.ID,
		Name:                payload.Name,
		Provider:            payload.Provider,
		Protocol:            domainmodel.Protocol(payload.Protocol),
		AuthType:            domainmodel.AuthType(payload.AuthType),
		BaseURL:             payload.BaseURL,
		APIKey:              payload.APIKey,
		ModelName:           payload.ModelName,
		ContextWindowTokens: optionalInt(payload.ContextWindowTokens),
		MaxOutputTokens:     optionalInt(payload.MaxOutputTokens),
		IsDefault:           payload.IsDefault,
		DefaultGenerationSettings: generation.Settings{
			Temperature:     payload.Defaults.Temperature,
			TopP:            payload.Defaults.TopP,
			MaxOutputTokens: payload.Defaults.MaxOutputTokens,
		},
	}); err != nil {
		return err
	}

	var added protocol.ModelSummary
	models := modelSummaries(a.chatService.ListModels())
	for _, model := range models {
		if model.ID == payload.ID {
			added = model
			break
		}
	}
	return a.writeRemoteMessage(conn, protocol.TypeModelConfigAddResult, message.RequestID, message.SessionID, protocol.ModelConfigAddResultPayload{
		Model:   added,
		Models:  models,
		Message: fmt.Sprintf("Model %s added.", payload.ID),
	})
}

func (a *Agent) handleModelConfigTest(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelConfigTestPayload](message)
	if err != nil {
		return fmt.Errorf("decode model config test failed: %w", err)
	}

	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	latency, err := a.chatService.TestModelConfig(ctx, modelcommand.AddConfig{
		ID:                  payload.ID,
		Name:                payload.Name,
		Provider:            payload.Provider,
		Protocol:            domainmodel.Protocol(payload.Protocol),
		AuthType:            domainmodel.AuthType(payload.AuthType),
		BaseURL:             payload.BaseURL,
		APIKey:              payload.APIKey,
		ModelName:           payload.ModelName,
		ContextWindowTokens: optionalInt(payload.ContextWindowTokens),
		MaxOutputTokens:     optionalInt(payload.MaxOutputTokens),
		DefaultGenerationSettings: generation.Settings{
			Temperature:     payload.Defaults.Temperature,
			TopP:            payload.Defaults.TopP,
			MaxOutputTokens: payload.Defaults.MaxOutputTokens,
		},
	})
	if err != nil {
		return err
	}

	return a.writeRemoteMessage(conn, protocol.TypeModelConfigTestResult, message.RequestID, message.SessionID, protocol.ModelConfigTestResultPayload{
		Success:   true,
		LatencyMS: latency,
		Message:   fmt.Sprintf("Model connection test succeeded (%d ms).", latency),
	})
}

func (a *Agent) handleModelConfigUpdate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelConfigUpdatePayload](message)
	if err != nil {
		return fmt.Errorf("decode model config update failed: %w", err)
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := a.chatService.UpdateModelConfig(ctx, modelcommand.UpdateConfig{
		ID: payload.ID, Name: payload.Name, Provider: payload.Provider,
		Protocol: domainmodel.Protocol(payload.Protocol), AuthType: domainmodel.AuthType(payload.AuthType),
		BaseURL: payload.BaseURL, APIKey: payload.APIKey, ModelName: payload.ModelName,
		ContextWindowTokens: payload.ContextWindowTokens, MaxOutputTokens: payload.MaxOutputTokens,
		DefaultGenerationSettings: generation.Settings{Temperature: payload.Defaults.Temperature, TopP: payload.Defaults.TopP, MaxOutputTokens: payload.Defaults.MaxOutputTokens},
	}); err != nil {
		return err
	}
	return a.writeModelMutation(conn, message, payload.ID, "Model updated.", "")
}

func (a *Agent) handleModelConfigDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelConfigDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode model config delete failed: %w", err)
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := a.chatService.DeleteModelConfig(ctx, modelcommand.DeleteConfig{ID: payload.ID}); err != nil {
		return err
	}
	return a.writeModelMutation(conn, message, "", fmt.Sprintf("Model %s deleted.", payload.ID), payload.ID)
}

func (a *Agent) handleModelConfigEnabledSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelConfigEnabledSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode model enabled set failed: %w", err)
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := a.chatService.SetModelEnabled(ctx, modelcommand.SetEnabled{ID: payload.ID, Enabled: payload.Enabled}); err != nil {
		return err
	}
	return a.writeModelMutation(conn, message, payload.ID, fmt.Sprintf("Model %s %s.", payload.ID, enabledLabel(payload.Enabled)), "")
}

func (a *Agent) handleModelConfigDefaultSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.ModelConfigDefaultSetPayload](message)
	if err != nil {
		return fmt.Errorf("decode model default set failed: %w", err)
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := a.chatService.SetDefaultModel(ctx, modelcommand.SetDefault{ID: payload.ID}); err != nil {
		return err
	}
	return a.writeModelMutation(conn, message, payload.ID, fmt.Sprintf("Model %s is now the default model.", payload.ID), "")
}

func (a *Agent) writeModelMutation(conn *websocket.Conn, message protocol.Message, modelID, text, deletedID string) error {
	var selected *protocol.ModelSummary
	models := modelSummaries(a.chatService.ListModels())
	for index := range models {
		if models[index].ID == modelID {
			selected = &models[index]
			break
		}
	}
	return a.writeRemoteMessage(conn, protocol.TypeModelConfigMutationResult, message.RequestID, message.SessionID, protocol.ModelConfigMutationResultPayload{
		Model: selected, DeletedID: deletedID, Models: models, Message: text,
	})
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func (a *Agent) skillListPayload(ctx context.Context, reloaded bool) (protocol.SkillListResultPayload, error) {
	var skills []skill.Skill
	var err error
	if reloaded {
		skills, err = a.chatService.ReloadSkills(ctx, "remote_reload")
	} else {
		skills, err = a.chatService.ListSkills(ctx)
	}
	if err != nil {
		return protocol.SkillListResultPayload{}, err
	}

	root := a.chatService.SkillRoot()
	message := ""
	if reloaded {
		message = fmt.Sprintf("Reloaded %d local skill(s).", len(skills))
	}
	if len(skills) == 0 {
		message = "No local skills found. Install one with SkillHub or create skills/<name>/SKILL.md."
	}

	return protocol.SkillListResultPayload{
		Root:     filepath.ToSlash(root),
		Skills:   skillSummaries(root, skills),
		Count:    len(skills),
		Reloaded: reloaded,
		Message:  message,
	}, nil
}
