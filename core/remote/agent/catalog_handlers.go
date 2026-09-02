package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"

	modelcommand "myai/core/application/model/command"
	"myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
	pluginruntime "myai/core/plugin"
	"myai/core/remote/protocol"
	"myai/core/skill"
)

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
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := a.pluginManager.Reload(ctx); err != nil {
		return err
	}
	payload := a.pluginListPayload(true, "插件已重载。")
	return a.writeRemoteMessage(conn, protocol.TypePluginReloadResult, message.RequestID, message.SessionID, payload)
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
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
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
		ID:        payload.ID,
		Name:      payload.Name,
		Provider:  payload.Provider,
		Protocol:  domainmodel.Protocol(payload.Protocol),
		AuthType:  domainmodel.AuthType(payload.AuthType),
		BaseURL:   payload.BaseURL,
		APIKey:    payload.APIKey,
		ModelName: payload.ModelName,
		IsDefault: payload.IsDefault,
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
		ID:        payload.ID,
		Name:      payload.Name,
		Provider:  payload.Provider,
		Protocol:  domainmodel.Protocol(payload.Protocol),
		AuthType:  domainmodel.AuthType(payload.AuthType),
		BaseURL:   payload.BaseURL,
		APIKey:    payload.APIKey,
		ModelName: payload.ModelName,
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
