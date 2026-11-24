package agent

import (
	"context"
	"errors"

	"github.com/gorilla/websocket"

	imageresult "myai/core/application/imagemodel/result"

	domain "myai/core/domain/imagemodel"
	"myai/core/remote/protocol"
)

func (a *Agent) handleImageModelList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	models, defaultID, err := a.chatService.ListImageModelConfigs(ctx)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeImageModelListResult, message.RequestID, message.SessionID, protocol.ImageModelListResultPayload{Models: protocol.NewImageModelSummaries(models), DefaultModelID: defaultID})
}
func imageConfigFromAdd(p protocol.ImageModelConfigAddPayload) domain.Config {
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	return domain.Config{ID: p.ID, Name: p.Name, Provider: p.Provider, Protocol: domain.Protocol(p.Protocol), AuthType: domain.AuthType(p.AuthType), BaseURL: p.BaseURL, APIKey: p.APIKey, ModelName: p.ModelName, Enabled: enabled, IsDefault: p.IsDefault}
}
func imageConfigFromUpdate(p protocol.ImageModelConfigUpdatePayload) domain.Config {
	return domain.Config{ID: p.ID, Name: p.Name, Provider: p.Provider, Protocol: domain.Protocol(p.Protocol), AuthType: domain.AuthType(p.AuthType), BaseURL: p.BaseURL, APIKey: p.APIKey, ModelName: p.ModelName}
}
func imageConfigFromTest(p protocol.ImageModelConfigTestPayload) domain.Config {
	return domain.Config{ID: p.ID, Protocol: domain.Protocol(p.Protocol), AuthType: domain.AuthType(p.AuthType), BaseURL: p.BaseURL, APIKey: p.APIKey, ModelName: p.ModelName}
}
func (a *Agent) handleImageModelConfigAdd(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	p, err := protocol.DecodePayload[protocol.ImageModelConfigAddPayload](message)
	if err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	result, err := a.chatService.AddImageModelConfig(ctx, imageConfigFromAdd(p))
	if err != nil {
		return err
	}
	if result.Model == nil {
		return errors.New("image model mutation returned no model")
	}
	return a.writeRemoteMessage(conn, protocol.TypeImageModelConfigAddResult, message.RequestID, message.SessionID, protocol.ImageModelConfigAddResultPayload{Model: protocol.NewImageModelSummary(*result.Model), Models: protocol.NewImageModelSummaries(result.Models), DefaultModelID: result.DefaultModelID})
}
func (a *Agent) handleImageModelConfigTest(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	p, err := protocol.DecodePayload[protocol.ImageModelConfigTestPayload](message)
	if err != nil {
		return err
	}
	result, testErr := a.chatService.TestImageModelConfig(ctx, imageConfigFromTest(p))
	response := protocol.ImageModelConfigTestResultPayload{OK: testErr == nil && result.OK, Message: result.Message, LatencyMS: result.LatencyMS, MIMEType: result.MIMEType}
	if testErr != nil {
		if response.Message == "" {
			response.Message = "Image model test failed. Check the endpoint, model and credentials."
		}
	}
	if response.Message == "" {
		if response.OK {
			response.Message = "Image model is available."
		} else {
			response.Message = "Image model test failed."
		}
	}
	return a.writeRemoteMessage(conn, protocol.TypeImageModelConfigTestResult, message.RequestID, message.SessionID, response)
}
func (a *Agent) handleImageModelConfigUpdate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	p, err := protocol.DecodePayload[protocol.ImageModelConfigUpdatePayload](message)
	if err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	result, err := a.chatService.UpdateImageModelConfig(ctx, imageConfigFromUpdate(p))
	if err != nil {
		return err
	}
	return a.writeImageModelMutation(conn, message, result)
}
func (a *Agent) handleImageModelConfigDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	p, err := protocol.DecodePayload[protocol.ImageModelConfigDeletePayload](message)
	if err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	result, err := a.chatService.DeleteImageModelConfig(ctx, p.ID)
	if err != nil {
		return err
	}
	return a.writeImageModelMutation(conn, message, result)
}
func (a *Agent) handleImageModelConfigEnabledSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	p, err := protocol.DecodePayload[protocol.ImageModelConfigEnabledSetPayload](message)
	if err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	result, err := a.chatService.SetImageModelEnabled(ctx, p.ID, p.Enabled)
	if err != nil {
		return err
	}
	return a.writeImageModelMutation(conn, message, result)
}
func (a *Agent) handleImageModelConfigDefaultSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateImageModelConfigMessage(message); err != nil {
		return err
	}
	p, err := protocol.DecodePayload[protocol.ImageModelConfigDefaultSetPayload](message)
	if err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	result, err := a.chatService.SetDefaultImageModel(ctx, p.ID)
	if err != nil {
		return err
	}
	return a.writeImageModelMutation(conn, message, result)
}
func (a *Agent) writeImageModelMutation(conn *websocket.Conn, message protocol.Message, result imageresult.MutationResult) error {
	var model *protocol.ImageModelSummary
	if result.Model != nil {
		converted := protocol.NewImageModelSummary(*result.Model)
		model = &converted
	}
	return a.writeRemoteMessage(conn, protocol.TypeImageModelConfigMutationResult, message.RequestID, message.SessionID, protocol.ImageModelConfigMutationResultPayload{
		Action: result.Action, Model: model, Models: protocol.NewImageModelSummaries(result.Models), DefaultModelID: result.DefaultModelID, Message: result.Message,
	})
}
