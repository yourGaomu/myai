package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

type Config struct {
	ServerURL   string
	UserID      string
	DeviceID    string
	SessionID   string
	ClientToken string
	Message     string
	AllowTools  bool
}

type Client struct {
	config Config
}

func New(config Config) *Client {
	return &Client{config: config}
}

func (c *Client) Run(ctx context.Context) error {
	if c.config.ServerURL == "" {
		return fmt.Errorf("server url is empty")
	}
	if c.config.UserID == "" {
		return fmt.Errorf("user id is empty")
	}
	if c.config.DeviceID == "" {
		return fmt.Errorf("device id is empty")
	}
	if strings.TrimSpace(c.config.Message) == "" {
		return fmt.Errorf("message is empty")
	}

	conn, response, err := websocket.DefaultDialer.DialContext(ctx, c.config.ServerURL, nil)
	if err != nil {
		if response != nil {
			return fmt.Errorf("connect relay failed: %w, status: %s", err, response.Status)
		}
		return fmt.Errorf("connect relay failed: %w", err)
	}
	defer conn.Close()

	requestID := newRequestID()
	message, err := protocol.NewMessage(
		protocol.TypeUserMessage,
		requestID,
		c.config.UserID,
		c.config.DeviceID,
		c.config.SessionID,
		protocol.UserMessagePayload{Content: c.config.Message},
	)
	if err != nil {
		return err
	}
	message.ClientToken = c.config.ClientToken

	fmt.Println("client connected.")
	fmt.Println("request:", requestID)
	if err := conn.WriteJSON(message); err != nil {
		return fmt.Errorf("send user message failed: %w", err)
	}

	return c.readLoop(ctx, conn, requestID)
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, requestID string) error {
	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client stopped"))
			return nil
		default:
		}

		var message protocol.Message
		if err := conn.ReadJSON(&message); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("read relay message failed: %w", err)
		}

		if message.RequestID != "" && message.RequestID != requestID {
			continue
		}

		switch message.Type {
		case protocol.TypeHeartbeat:
			fmt.Println("relay ack:", message.RequestID)
		case protocol.TypeAssistantDelta:
			payload, err := protocol.DecodePayload[protocol.AssistantDeltaPayload](message)
			if err != nil {
				return fmt.Errorf("decode assistant delta failed: %w", err)
			}
			fmt.Print(payload.Content)
		case protocol.TypeAssistantDone:
			payload, err := protocol.DecodePayload[protocol.AssistantDonePayload](message)
			if err != nil {
				return fmt.Errorf("decode assistant done failed: %w", err)
			}
			if payload.Content != "" {
				fmt.Println()
			}
			fmt.Println("done.")
			return nil
		case protocol.TypeToolCall:
			payload, err := protocol.DecodePayload[protocol.ToolCallPayload](message)
			if err != nil {
				return fmt.Errorf("decode tool call failed: %w", err)
			}
			fmt.Printf("\ntool call: %s %s\n", payload.Name, payload.Arguments)
		case protocol.TypePermissionAsk:
			payload, err := protocol.DecodePayload[protocol.PermissionAskPayload](message)
			if err != nil {
				return fmt.Errorf("decode permission ask failed: %w", err)
			}
			decision := "denied"
			if c.config.AllowTools {
				decision = "allowed"
			}
			fmt.Printf("\npermission required: %s needs %s (%s)\n", payload.Name, payload.Permission, decision)

			reply, err := protocol.NewMessage(
				protocol.TypePermissionResult,
				requestID,
				c.config.UserID,
				c.config.DeviceID,
				message.SessionID,
				protocol.PermissionResultPayload{Allowed: c.config.AllowTools},
			)
			if err != nil {
				return err
			}
			reply.ClientToken = c.config.ClientToken
			if err := conn.WriteJSON(reply); err != nil {
				return fmt.Errorf("send permission result failed: %w", err)
			}
		case protocol.TypeError:
			payload, err := protocol.DecodePayload[protocol.ErrorPayload](message)
			if err != nil {
				return fmt.Errorf("decode remote error failed: %w", err)
			}
			return fmt.Errorf("remote error: %s", payload.Message)
		default:
			fmt.Printf("\nmessage: type=%s request=%s\n", message.Type, message.RequestID)
		}
	}
}

func newRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
