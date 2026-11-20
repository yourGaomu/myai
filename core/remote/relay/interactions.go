package relay

import (
	"encoding/json"
	"errors"
	"time"

	"myai/core/remote/protocol"
)

const pendingInteractionLimit = 1024

const (
	// Match the Agent's foreground approval wait. Limit additional retention
	// independently of background interaction replay and the request route TTL.
	foregroundPermissionTTL      = time.Minute
	foregroundPermissionLimit    = 128
	foregroundPermissionFrameMax = 64 * 1024
)

type foregroundPermissionKey struct {
	route      *clientEntry
	approvalID string
}

type foregroundPermission struct {
	key       foregroundPermissionKey
	message   protocol.Message
	expiresAt time.Time
}

// trackForegroundPermissionLocked runs during request correlation, including
// when the paired client is offline. Foreground asks never enter the event
// broadcast cache. The route owns the original token hash for its lifetime.
func (s *Server) trackForegroundPermissionLocked(route *clientEntry, message protocol.Message, terminal bool, now time.Time) {
	if !isForegroundChatRequest(route.RequestType) {
		return
	}
	if terminal || message.Type == protocol.TypeAssistantDone {
		route.foregroundFinished = true
		s.clearForegroundPermissionsLocked(route)
		return
	}
	if route.foregroundFinished || route.UserID != message.UserID || route.DeviceID != message.DeviceID {
		return
	}
	switch message.Type {
	case protocol.TypePermissionResult:
		// The Agent echoes a successful result after consuming the approval.
		// Rejected control replies can leave the approval pending.
		delete(s.foregroundPermissions, foregroundPermissionKey{route, controlReplyID(message, "approval_id")})
	case protocol.TypePermissionAsk:
		if len(message.Payload)+len(message.RequestID)+len(message.SessionID)+len(message.UserID)+len(message.DeviceID)+len(message.ClientToken) > foregroundPermissionFrameMax {
			return
		}
		approvalID := controlReplyID(message, "approval_id")
		if approvalID == "" {
			return
		}
		key := foregroundPermissionKey{route, approvalID}
		for candidate, pending := range s.foregroundPermissions {
			if !now.Before(pending.expiresAt) {
				delete(s.foregroundPermissions, candidate)
			}
		}
		if len(s.foregroundPermissions) < foregroundPermissionLimit && s.foregroundPermissions[key] == nil {
			s.foregroundPermissions[key] = &foregroundPermission{key: key, message: message, expiresAt: now.Add(foregroundPermissionTTL)}
		}
	}
}

func isForegroundChatRequest(kind protocol.MessageType) bool {
	switch kind {
	case protocol.TypeUserMessage, protocol.TypeSessionRegenerate, protocol.TypeSessionPlanExecute, protocol.TypeSubagentTaskResume:
		return true
	default:
		return false
	}
}

func (s *Server) clearForegroundPermissionsLocked(route *clientEntry) {
	for key := range s.foregroundPermissions {
		if key.route == route {
			delete(s.foregroundPermissions, key)
		}
	}
}

func (s *Server) reattachForegroundPermissionsLocked(p *peer, identity *clientConnection) []*foregroundPermission {
	var messages []*foregroundPermission
	now := time.Now()
	for key, pending := range s.foregroundPermissions {
		if !now.Before(pending.expiresAt) || now.Sub(key.route.LastSeenAt) > clientRouteTTL {
			delete(s.foregroundPermissions, key)
			continue
		}
		if key.route.ownsConnection(identity) {
			// Route control replies and chat output to the socket that is about
			// to receive the prompt, even if the old socket has not closed yet.
			key.route.peer = p
			key.route.LastSeenAt = now
			messages = append(messages, pending)
		}
	}
	return messages
}

func (s *Server) replayForegroundPermissions(p *peer, pending []*foregroundPermission) {
	for _, permission := range pending {
		if err := s.writeRoutedClient(p, permission.key.route, permission.message, permission); err != nil {
			return
		}
	}
}

type interactionKey struct {
	UserID, DeviceID, SessionID, TurnID, Kind, ID string
}

type pendingInteraction struct {
	Message   protocol.Message
	Clients   map[string]struct{}
	ExpiresAt time.Time
}

func interactionFromMessage(message protocol.Message) (interactionKey, bool, bool) {
	key := interactionKey{UserID: message.UserID, DeviceID: message.DeviceID, SessionID: message.SessionID, TurnID: message.RequestID}
	resolved := false
	switch message.Type {
	case protocol.TypeBackgroundTurnEvent:
		payload, err := protocol.DecodePayload[protocol.BackgroundTurnEventPayload](message)
		if err != nil {
			return key, false, false
		}
		key.TurnID, key.SessionID = payload.TurnID, payload.SessionID
		switch payload.Kind {
		case "permission_ask", "permission_resolved":
			key.Kind, key.ID = "permission", payload.ApprovalID
			if key.ID == "" && payload.Permission != nil {
				key.ID = payload.Permission.ApprovalID
			}
			resolved = payload.Kind == "permission_resolved"
		case "user_question", "user_question_resolved":
			key.Kind = "question"
			if payload.Question != nil {
				key.ID = payload.Question.QuestionID
			}
			resolved = payload.Kind == "user_question_resolved"
		default:
			return key, false, false
		}
	case protocol.TypeUserQuestion:
		key.Kind, key.ID = "question", controlReplyID(message, "question_id")
	case protocol.TypeUserQuestionResolved:
		key.Kind, key.ID, resolved = "question", controlReplyID(message, "question_id"), true
	default:
		return key, false, false
	}
	return key, key.TurnID != "" && key.ID != "", resolved
}

func (s *Server) rememberInteraction(message protocol.Message) error {
	key, ok, resolved := interactionFromMessage(message)
	now := time.Now()
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	for candidate, pending := range s.interactions {
		if !now.Before(pending.ExpiresAt) {
			delete(s.interactions, candidate)
		}
	}
	if message.Type == protocol.TypeBackgroundTurnEvent {
		var payload protocol.BackgroundTurnEventPayload
		if json.Unmarshal(message.Payload, &payload) == nil && (payload.Kind == "completed" || payload.Kind == "resync_required") {
			for candidate, pending := range s.interactions {
				if candidate.UserID == message.UserID && candidate.DeviceID == message.DeviceID && (candidate.TurnID == payload.TurnID || payload.Kind == "resync_required" && pending.Message.Type == protocol.TypeBackgroundTurnEvent) {
					delete(s.interactions, candidate)
				}
			}
		}
	}
	if !ok {
		return nil
	}
	if resolved {
		delete(s.interactions, key)
		return nil
	}
	if existing := s.interactions[key]; existing != nil {
		existing.Message = message
		return nil
	}
	if len(s.interactions) >= pendingInteractionLimit {
		return errors.New("too many pending remote interactions")
	}
	s.interactions[key] = &pendingInteraction{Message: message, Clients: make(map[string]struct{}), ExpiresAt: now.Add(clientRouteTTL)}
	return nil
}

// Grant only to an authenticated identity that was a delivery target. The same
// paired token can reconnect; another token must first receive the prompt.
func (s *Server) grantInteraction(p *peer, message protocol.Message) {
	key, ok, resolved := interactionFromMessage(message)
	if !ok || resolved {
		return
	}
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	pending, connection := s.interactions[key], s.connections[p]
	if pending != nil && connection != nil && connection.UserID == key.UserID && connection.DeviceID == key.DeviceID {
		pending.Clients[connection.ClientID] = struct{}{}
	}
}

func (s *Server) authorizeInteraction(p *peer, message protocol.Message, kind, id string) bool {
	key := interactionKey{UserID: message.UserID, DeviceID: message.DeviceID, SessionID: message.SessionID, TurnID: message.RequestID, Kind: kind, ID: id}
	s.clientLock.RLock()
	defer s.clientLock.RUnlock()
	pending, connection := s.interactions[key], s.connections[p]
	if pending == nil || connection == nil || !time.Now().Before(pending.ExpiresAt) || connection.ClientID != clientTokenHash(message.ClientToken) {
		return false
	}
	_, granted := pending.Clients[connection.ClientID]
	return granted
}

func (s *Server) replayInteractions(p *peer, userID, deviceID string) {
	s.clientLock.RLock()
	messages := make([]protocol.Message, 0)
	for key, pending := range s.interactions {
		if key.UserID == userID && key.DeviceID == deviceID && time.Now().Before(pending.ExpiresAt) {
			messages = append(messages, pending.Message)
		}
	}
	s.clientLock.RUnlock()
	for _, message := range messages {
		s.grantInteraction(p, message)
		if err := s.writeAuthorizedClient(p, message); err != nil {
			return
		}
	}
}

func (s *Server) forwardControlReply(message protocol.Message) error {
	if s.getClient(message.RequestID) != nil {
		return s.forwardToClient(message)
	}
	// Background turns have no foreground request route. Their prompt grants
	// supply the response targets, without creating or completing a chat route.
	kind, id := "permission", controlReplyID(message, "approval_id")
	if id == "" {
		kind, id = "question", controlReplyID(message, "question_id")
	}
	key := interactionKey{UserID: message.UserID, DeviceID: message.DeviceID, SessionID: message.SessionID, TurnID: message.RequestID, Kind: kind, ID: id}
	s.clientLock.RLock()
	var targets []*peer
	if pending := s.interactions[key]; pending != nil {
		for p, connection := range s.connections {
			if connection.UserID == key.UserID && connection.DeviceID == key.DeviceID {
				if _, ok := pending.Clients[connection.ClientID]; ok {
					targets = append(targets, p)
				}
			}
		}
	}
	s.clientLock.RUnlock()
	for _, p := range targets {
		if err := s.writeAuthorizedClient(p, message); err != nil {
			return err
		}
	}
	return nil
}
