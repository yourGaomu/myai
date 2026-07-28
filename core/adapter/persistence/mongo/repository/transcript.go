package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	"myai/core/adapter/persistence/mongo/po"
	portrepository "myai/core/port/repository"
)

func (m *Store) ReplaceSessionTranscript(ctx context.Context, snapshot portrepository.TranscriptSnapshot) error {
	if m == nil || m.database == nil {
		return errors.New("mongo transcript repository is not configured")
	}
	sessionID := strings.TrimSpace(snapshot.Session.ID)
	if sessionID == "" {
		return errors.New("transcript session id is empty")
	}
	for _, message := range snapshot.Messages {
		if strings.TrimSpace(message.ID) == "" {
			return errors.New("transcript message id is empty")
		}
		if message.SessionID != sessionID {
			return fmt.Errorf("transcript message %q belongs to session %q instead of %q", message.ID, message.SessionID, sessionID)
		}
	}

	mongoSession, err := m.database.Client().StartSession()
	if err != nil {
		return fmt.Errorf("start transcript transaction: %w", err)
	}
	defer mongoSession.EndSession(context.Background())

	_, err = mongoSession.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		oldToolCallIDs, err := m.transcriptToolCallIDs(transactionContext, sessionID)
		if err != nil {
			return nil, err
		}
		retainedToolCallIDs := toolCallIDs(snapshot.Messages)

		if _, err := m.database.Collection(messagesCollection).DeleteMany(transactionContext, bson.M{"session_id": sessionID}); err != nil {
			return nil, fmt.Errorf("clear regenerated transcript messages: %w", err)
		}
		if len(snapshot.Messages) > 0 {
			documents := make([]any, 0, len(snapshot.Messages))
			for _, message := range snapshot.Messages {
				documents = append(documents, mongomapper.MessageDocumentFromRecord(message))
			}
			if _, err := m.database.Collection(messagesCollection).InsertMany(transactionContext, documents); err != nil {
				return nil, fmt.Errorf("write regenerated transcript messages: %w", err)
			}
		}

		removed := removedToolCallIDs(oldToolCallIDs, retainedToolCallIDs)
		if len(removed) > 0 {
			filter := bson.M{"session_id": sessionID, "tool_call_id": bson.M{"$in": removed}}
			if _, err := m.database.Collection(assetsCollection).DeleteMany(transactionContext, filter); err != nil {
				return nil, fmt.Errorf("remove regenerated transcript assets: %w", err)
			}
		}
		if err := m.SaveSession(transactionContext, snapshot.Session); err != nil {
			return nil, fmt.Errorf("save regenerated session: %w", err)
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("replace regenerated transcript: %w", err)
	}
	return nil
}

func (m *Store) transcriptToolCallIDs(ctx context.Context, sessionID string) ([]string, error) {
	var documents []po.ToolCallReferenceDocument
	cursor, err := m.database.Collection(messagesCollection).Find(
		ctx,
		bson.M{"session_id": sessionID, "role": portrepository.RoleToolCall, "tool_call_id": bson.M{"$ne": ""}},
		options.Find().SetProjection(bson.M{"_id": 0, "tool_call_id": 1}),
	)
	if err != nil {
		return nil, fmt.Errorf("list existing transcript tool calls: %w", err)
	}
	defer cursor.Close(ctx)
	if err := cursor.All(ctx, &documents); err != nil {
		return nil, fmt.Errorf("decode existing transcript tool calls: %w", err)
	}
	result := make([]string, 0, len(documents))
	for _, document := range documents {
		if id := strings.TrimSpace(document.ToolCallID); id != "" {
			result = append(result, id)
		}
	}
	return result, nil
}

func toolCallIDs(messages []portrepository.MessageRecord) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range messages {
		if message.Role != portrepository.RoleToolCall {
			continue
		}
		id := strings.TrimSpace(message.ToolCallID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func removedToolCallIDs(previous []string, retained []string) bson.A {
	keep := make(map[string]struct{}, len(retained))
	for _, id := range retained {
		if id = strings.TrimSpace(id); id != "" {
			keep[id] = struct{}{}
		}
	}
	removed := bson.A{}
	seen := make(map[string]struct{})
	for _, id := range previous {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := keep[id]; exists {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		removed = append(removed, id)
	}
	return removed
}

var _ portrepository.TranscriptRepository = (*Store)(nil)
