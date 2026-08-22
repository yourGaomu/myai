package agent

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gorilla/websocket"

	modelcommand "myai/core/application/model/command"
	"myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
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
