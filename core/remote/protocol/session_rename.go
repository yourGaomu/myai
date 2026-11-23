package protocol

import (
	"errors"
	"fmt"
	"strings"
)

func SessionRenameRequest(message Message) (SessionRenamePayload, error) {
	if strings.TrimSpace(message.RequestID) == "" {
		return SessionRenamePayload{}, errors.New("session rename requires a request id")
	}
	payload, err := DecodePayload[SessionRenamePayload](message)
	if err != nil {
		return SessionRenamePayload{}, fmt.Errorf("decode session rename failed: %w", err)
	}
	if payload.SessionID == "" || payload.SessionID != strings.TrimSpace(payload.SessionID) {
		return SessionRenamePayload{}, errors.New("session rename requires an explicit payload session id")
	}
	if message.SessionID != "" && message.SessionID != payload.SessionID {
		return SessionRenamePayload{}, errors.New("session rename envelope and payload session ids do not match")
	}
	return payload, nil
}
