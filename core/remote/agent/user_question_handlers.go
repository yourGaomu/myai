package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	domaininteraction "myai/core/domain/interaction"
	"myai/core/remote/protocol"
)

func (a *Agent) handleUserQuestionAnswer(message protocol.Message) error {
	return a.handleUserQuestionAnswerContext(context.Background(), message)
}

func (a *Agent) handleUserQuestionAnswerContext(ctx context.Context, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.UserQuestionAnswerPayload](message)
	if err != nil {
		return fmt.Errorf("decode user question answer failed: %w", err)
	}
	if payload.QuestionID == "" {
		return errors.New("question_id is required")
	}
	answer := domaininteraction.Answer{Answers: payload.Answers, Cancelled: payload.Cancelled}
	if a.userQuestionWaiters.resolveFor(message.SessionID, message.RequestID, payload.QuestionID, answer) {
		return nil
	}
	if control, ok := a.chatService.(BackgroundQuestionControl); ok {
		return control.ResolveBackgroundQuestion(ctx, message.SessionID, message.RequestID, payload.QuestionID, answer)
	}
	return errors.New("user question is invalid, expired or already answered")
}

func (a *Agent) handleUserQuestionList(conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.UserQuestionListPayload](message)
	if err != nil {
		return fmt.Errorf("decode user question list failed: %w", err)
	}
	requests, resolvedIDs := a.userQuestionWaiters.list(payload.SessionID, payload.KnownQuestionIDs)
	if source, ok := a.chatService.(BackgroundQuestionSource); ok {
		background := source.ListBackgroundQuestions("")
		active := make(map[string]struct{}, len(background))
		for _, request := range background {
			active[request.ID] = struct{}{}
			if payload.SessionID == "" || request.SessionID == payload.SessionID {
				requests = append(requests, request)
			}
		}
		filtered := resolvedIDs[:0]
		for _, id := range resolvedIDs {
			if _, pending := active[id]; !pending {
				filtered = append(filtered, id)
			}
		}
		resolvedIDs = filtered
	}
	items := make([]protocol.UserQuestionListItemPayload, 0, len(requests))
	for _, request := range requests {
		items = append(items, protocol.UserQuestionListItemPayload{
			SessionID: request.SessionID, RequestID: request.RequestID, Question: userQuestionPayload(request),
		})
	}
	return a.writeRemoteMessage(conn, protocol.TypeUserQuestionListResult, message.RequestID, message.SessionID,
		protocol.UserQuestionListResultPayload{Questions: items, ResolvedQuestionIDs: resolvedIDs})
}

func (a *Agent) askUserQuestion(ctx context.Context, conn *websocket.Conn, message protocol.Message, request domaininteraction.Request) (domaininteraction.Answer, error) {
	if request.SessionID == "" {
		request.SessionID = message.SessionID
	}
	if request.RequestID == "" {
		request.RequestID = message.RequestID
	}
	if err := domaininteraction.ValidateRequest(request); err != nil {
		return domaininteraction.Answer{}, err
	}
	waiter, err := a.userQuestionWaiters.register(request)
	if err != nil {
		return domaininteraction.Answer{}, err
	}
	defer a.userQuestionWaiters.unregister(request.ID, waiter)

	payload := userQuestionPayload(request)
	if err := a.writeRemoteMessage(conn, protocol.TypeUserQuestion, message.RequestID, message.SessionID, payload); err != nil {
		return domaininteraction.Answer{}, err
	}
	timeout := a.userQuestionTimeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	status := "canceled"
	defer func() {
		_ = a.writeRemoteMessage(conn, protocol.TypeUserQuestionResolved, message.RequestID, message.SessionID,
			protocol.UserQuestionResolvedPayload{QuestionID: request.ID, Status: status})
	}()
	select {
	case answer := <-waiter:
		if answer.Cancelled {
			status = "dismissed"
			return answer, nil
		}
		status = "answered"
		return answer, nil
	case <-timer.C:
		a.userQuestionWaiters.unregister(request.ID, waiter)
		status = "timed_out"
		return domaininteraction.Answer{}, errors.New("user question timed out")
	case <-ctx.Done():
		a.userQuestionWaiters.unregister(request.ID, waiter)
		status = "canceled"
		return domaininteraction.Answer{}, ctx.Err()
	}
}

func userQuestionPayload(request domaininteraction.Request) protocol.UserQuestionPayload {
	questions := make([]protocol.UserQuestionItemPayload, 0, len(request.Questions))
	for _, question := range request.Questions {
		questions = append(questions, protocol.UserQuestionItemPayload{
			ID: question.ID, Prompt: question.Prompt, Options: question.Options, AllowOther: question.AllowOther,
		})
	}
	return protocol.UserQuestionPayload{
		QuestionID: request.ID, ToolCallID: request.ToolCallID, RunID: request.RunID,
		SourceTaskID: request.SourceTaskID, SourceSessionID: request.SourceSessionID,
		Title: request.Title, Questions: questions,
	}
}
