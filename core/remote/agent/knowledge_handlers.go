package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/gorilla/websocket"

	catalogcommand "myai/core/application/knowledge/catalog/command"
	documentcommand "myai/core/application/knowledge/document/command"
	querycommand "myai/core/application/knowledge/query/command"
	searchcommand "myai/core/application/knowledge/search/command"
	"myai/core/remote/protocol"
)

func (a *Agent) requireKnowledgeService() error {
	if a.knowledgeService == nil {
		return errors.New("knowledge service is not configured")
	}
	return nil
}

func (a *Agent) handleKnowledgeCatalogList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireKnowledgeService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.KnowledgeCatalogListPayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge catalog list failed: %w", err)
	}
	result, err := a.knowledgeService.ListCatalog(ctx, payload.IncludeDeleted)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeKnowledgeCatalogListResult, message.RequestID, message.SessionID, knowledgeCatalogPayload(result, ""))
}

func (a *Agent) handleKnowledgeCategoryCreate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeCategoryCreatePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge category create failed: %w", err)
	}
	return a.mutateKnowledgeCatalog(ctx, conn, message, "Category created.", func() error {
		_, err := a.knowledgeService.CreateCategory(ctx, catalogcommand.CreateCategory{Name: payload.Name, ParentID: payload.ParentID, SortOrder: payload.SortOrder})
		return err
	})
}

func (a *Agent) handleKnowledgeCategoryMove(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeCategoryMovePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge category move failed: %w", err)
	}
	return a.mutateKnowledgeCatalog(ctx, conn, message, "Category moved.", func() error {
		_, err := a.knowledgeService.MoveCategory(ctx, catalogcommand.MoveCategory{CategoryID: payload.CategoryID, ParentID: payload.ParentID, SortOrder: payload.SortOrder})
		return err
	})
}

func (a *Agent) handleKnowledgeCategoryDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeCategoryDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge category delete failed: %w", err)
	}
	return a.mutateKnowledgeCatalog(ctx, conn, message, "Category deleted.", func() error {
		return a.knowledgeService.DeleteCategory(ctx, catalogcommand.DeleteCategory{CategoryID: payload.CategoryID, Reason: payload.Reason, Recursive: payload.Recursive})
	})
}

func (a *Agent) handleKnowledgeBaseCreate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeBaseCreatePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge base create failed: %w", err)
	}
	return a.mutateKnowledgeCatalog(ctx, conn, message, "Knowledge base created.", func() error {
		_, err := a.knowledgeService.CreateKnowledgeBase(ctx, catalogcommand.CreateKnowledgeBase{
			CategoryID: payload.CategoryID, Name: payload.Name, Description: payload.Description,
			RAGEnabled: payload.RAGEnabled, ActiveIndexProfileID: payload.ActiveIndexProfileID,
		})
		return err
	})
}

func (a *Agent) handleKnowledgeBaseUpdate(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeBaseUpdatePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge base update failed: %w", err)
	}
	return a.mutateKnowledgeCatalog(ctx, conn, message, "Knowledge base updated.", func() error {
		_, err := a.knowledgeService.UpdateKnowledgeBase(ctx, catalogcommand.UpdateKnowledgeBase{
			KnowledgeBaseID: payload.KnowledgeBaseID, CategoryID: payload.CategoryID,
			Name: payload.Name, Description: payload.Description, RAGEnabled: payload.RAGEnabled,
			ActiveIndexProfileID: payload.ActiveIndexProfileID,
		})
		return err
	})
}

func (a *Agent) handleKnowledgeBaseDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeBaseDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge base delete failed: %w", err)
	}
	return a.mutateKnowledgeCatalog(ctx, conn, message, "Knowledge base deleted.", func() error {
		return a.knowledgeService.DeleteKnowledgeBase(ctx, catalogcommand.DeleteKnowledgeBase{KnowledgeBaseID: payload.KnowledgeBaseID, Reason: payload.Reason})
	})
}

func (a *Agent) mutateKnowledgeCatalog(ctx context.Context, conn *websocket.Conn, message protocol.Message, responseMessage string, mutate func() error) error {
	if err := a.requireKnowledgeService(); err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := mutate(); err != nil {
		return err
	}
	result, err := a.knowledgeService.ListCatalog(ctx, false)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeKnowledgeCatalogMutationResult, message.RequestID, message.SessionID, knowledgeCatalogPayload(result, responseMessage))
}

func (a *Agent) handleKnowledgeDocumentList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireKnowledgeService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.KnowledgeDocumentListPayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge document list failed: %w", err)
	}
	result, err := a.knowledgeService.ListDocuments(ctx, querycommand.Documents{KnowledgeBaseID: payload.KnowledgeBaseID, IncludeDeleted: payload.IncludeDeleted})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeKnowledgeDocumentListResult, message.RequestID, message.SessionID, knowledgeDocumentsPayload(payload.KnowledgeBaseID, result))
}

func (a *Agent) handleKnowledgeDocumentIngest(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeDocumentIngestPayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge document ingest failed: %w", err)
	}
	return a.mutateKnowledgeDocuments(ctx, conn, message, payload.KnowledgeBaseID, "Document accepted for indexing.", func() error {
		_, err := a.knowledgeService.IngestDocument(ctx, documentcommand.Ingest{KnowledgeBaseID: payload.KnowledgeBaseID, URL: payload.URL, Code: payload.Code})
		return err
	})
}

func (a *Agent) handleKnowledgeDocumentRetry(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeDocumentRetryPayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge document retry failed: %w", err)
	}
	return a.mutateKnowledgeDocuments(ctx, conn, message, payload.KnowledgeBaseID, "Indexing retry accepted.", func() error {
		_, err := a.knowledgeService.RetryDocument(ctx, documentcommand.Retry{JobID: payload.JobID})
		return err
	})
}

func (a *Agent) handleKnowledgeDocumentDelete(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.KnowledgeDocumentDeletePayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge document delete failed: %w", err)
	}
	return a.mutateKnowledgeDocuments(ctx, conn, message, payload.KnowledgeBaseID, "Document deleted.", func() error {
		return a.knowledgeService.DeleteDocument(ctx, documentcommand.Delete{DocumentID: payload.DocumentID, Reason: payload.Reason})
	})
}

func (a *Agent) mutateKnowledgeDocuments(ctx context.Context, conn *websocket.Conn, message protocol.Message, knowledgeBaseID string, responseMessage string, mutate func() error) error {
	if err := a.requireKnowledgeService(); err != nil {
		return err
	}
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if err := mutate(); err != nil {
		return err
	}
	result, err := a.knowledgeService.ListDocuments(ctx, querycommand.Documents{KnowledgeBaseID: knowledgeBaseID})
	if err != nil {
		return err
	}
	payload := knowledgeDocumentsPayload(knowledgeBaseID, result)
	payload.Message = responseMessage
	return a.writeRemoteMessage(conn, protocol.TypeKnowledgeDocumentMutationResult, message.RequestID, message.SessionID, payload)
}

func (a *Agent) handleKnowledgeProfileList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireKnowledgeService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.KnowledgeProfileListPayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge profile list failed: %w", err)
	}
	result, err := a.knowledgeService.ListIndexProfiles(ctx, payload.IncludeDeleted)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeKnowledgeProfileListResult, message.RequestID, message.SessionID, knowledgeProfilesPayload(result))
}

func (a *Agent) handleKnowledgeSearchPreview(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := a.requireKnowledgeService(); err != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.KnowledgeSearchPreviewPayload](message)
	if err != nil {
		return fmt.Errorf("decode knowledge search preview failed: %w", err)
	}
	result, err := a.knowledgeService.SearchKnowledge(ctx, searchcommand.Search{
		Text: payload.Query, KnowledgeBaseIDs: payload.KnowledgeBaseIDs,
		CategoryIDs: payload.CategoryIDs, TopK: payload.TopK, Strict: true,
	})
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeKnowledgeSearchPreviewResult, message.RequestID, message.SessionID, knowledgeSearchPayload(payload.Query, result))
}
