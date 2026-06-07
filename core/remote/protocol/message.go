package protocol

import "encoding/json"

type MessageType string

const (
	TypeAgentOnline      MessageType = "agent_online"
	TypeAgentOffline     MessageType = "agent_offline"
	TypeUserMessage      MessageType = "user_message"
	TypeAssistantDelta   MessageType = "assistant_delta"
	TypeAssistantDone    MessageType = "assistant_done"
	TypeToolCall         MessageType = "tool_call"
	TypePermissionAsk    MessageType = "permission_ask"
	TypePermissionResult MessageType = "permission_result"
	TypeError            MessageType = "error"
	TypeHeartbeat        MessageType = "heartbeat"
)

type Message struct {
	Type        MessageType     `json:"type"`
	RequestID   string          `json:"request_id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	DeviceID    string          `json:"device_id,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	ClientToken string          `json:"client_token,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

type AgentOnlinePayload struct {
	Status   string `json:"status"`
	BindCode string `json:"bind_code,omitempty"`
}

type UserMessagePayload struct {
	Content string `json:"content"`
}

type AssistantDeltaPayload struct {
	Content string `json:"content"`
}

type AssistantDonePayload struct {
	Content string `json:"content"`
}

type ToolCallPayload struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type PermissionAskPayload struct {
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Permission string `json:"permission"`
}

type PermissionResultPayload struct {
	Allowed bool `json:"allowed"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

func NewMessage(
	messageType MessageType,
	requestID string,
	userID string,
	deviceID string,
	sessionID string,
	payload any,
) (Message, error) {
	var raw json.RawMessage

	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return Message{}, err
		}
		raw = data
	}

	return Message{
		Type:      messageType,
		RequestID: requestID,
		UserID:    userID,
		DeviceID:  deviceID,
		SessionID: sessionID,
		Payload:   raw,
	}, nil
}

func DecodePayload[T any](message Message) (T, error) {
	var payload T
	if len(message.Payload) == 0 {
		return payload, nil
	}

	err := json.Unmarshal(message.Payload, &payload)
	return payload, err
}
