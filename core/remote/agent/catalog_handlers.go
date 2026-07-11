package agent

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gorilla/websocket"

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
