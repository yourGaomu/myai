import { useCallback } from "react";

import type {
  KnowledgeBase,
  KnowledgeBaseCreatePayload,
  KnowledgeBaseDeletePayload,
  KnowledgeBaseUpdatePayload,
  KnowledgeCategoryCreatePayload,
  KnowledgeCategoryDeletePayload,
  KnowledgeCategoryMovePayload,
  KnowledgeDocumentDeletePayload,
  KnowledgeDocumentIngestPayload,
  KnowledgeDocumentListPayload,
  KnowledgeDocumentRetryPayload,
  KnowledgeSearchPreviewPayload,
  RelayMessage,
  RAGSettings,
  SessionRAGSetPayload,
} from "../protocol";
import type { PendingAction } from "../types/app";
import type { KnowledgeBaseChanges } from "../types/knowledge";
import { newRequestID } from "../utils/ids";
import * as DocumentPicker from "expo-document-picker";
import { uploadMobileAsset } from "../utils/assetUpload";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  activeKnowledgeBase?: KnowledgeBase;
  assetBaseURL: string;
  clientToken: string;
  sessionID: string;
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
  onError: (message: string) => void;
};

export function useKnowledgeActions({
  activeKnowledgeBase,
  assetBaseURL,
  clientToken,
  sessionID,
  sendEnvelope,
  startPending,
  stopPending,
  onError,
}: Args) {
  const requestCatalog = useCallback(() => {
    if (!clientToken) return false;
    startPending("knowledge");
    if (!sendEnvelope("knowledge_catalog_list", { request_id: newRequestID(), payload: {} })) {
      stopPending("knowledge");
      return false;
    }
    return true;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const requestProfiles = useCallback(() => {
    if (!clientToken) return false;
    if (!sendEnvelope("knowledge_profile_list", { request_id: newRequestID(), payload: {} })) return false;
    return true;
  }, [clientToken, sendEnvelope]);

  const requestDocuments = useCallback((knowledgeBaseID = activeKnowledgeBase?.id || "") => {
    if (!clientToken || !knowledgeBaseID) return false;
    startPending("knowledge");
    const payload: KnowledgeDocumentListPayload = { knowledge_base_id: knowledgeBaseID };
    if (!sendEnvelope("knowledge_document_list", { request_id: newRequestID(), payload })) {
      stopPending("knowledge");
      return false;
    }
    return true;
  }, [activeKnowledgeBase?.id, clientToken, sendEnvelope, startPending, stopPending]);

  const createCategory = useCallback((name: string, parentID = "") => {
    if (!clientToken || !name.trim()) return false;
    startPending("knowledge");
    const payload: KnowledgeCategoryCreatePayload = { name: name.trim(), parent_id: parentID };
    const sent = sendEnvelope("knowledge_category_create", { request_id: newRequestID(), payload });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const moveCategory = useCallback((categoryID: string, parentID = "") => {
    if (!clientToken || !categoryID) return false;
    startPending("knowledge");
    const payload: KnowledgeCategoryMovePayload = { category_id: categoryID, parent_id: parentID };
    const sent = sendEnvelope("knowledge_category_move", {
      request_id: newRequestID(),
      payload,
    });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const deleteCategory = useCallback((categoryID: string, recursive = false) => {
    if (!clientToken || !categoryID) return false;
    startPending("knowledge");
    const payload: KnowledgeCategoryDeletePayload = { category_id: categoryID, recursive };
    const sent = sendEnvelope("knowledge_category_delete", {
      request_id: newRequestID(),
      payload,
    });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const createKnowledgeBase = useCallback((name: string, categoryID = "", activeIndexProfileID = "") => {
    if (!clientToken || !name.trim()) return false;
    startPending("knowledge");
    const payload: KnowledgeBaseCreatePayload = {
      name: name.trim(),
      category_id: categoryID,
      rag_enabled: Boolean(activeIndexProfileID),
      active_index_profile_id: activeIndexProfileID,
    };
    const sent = sendEnvelope("knowledge_base_create", {
      request_id: newRequestID(),
      payload,
    });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const updateKnowledgeBase = useCallback((base: KnowledgeBase, changes: KnowledgeBaseChanges) => {
    if (!clientToken) return false;
    const activeIndexProfileID = changes.activeIndexProfileID ?? base.active_index_profile_id ?? "";
    const ragEnabled = changes.ragEnabled ?? (changes.activeIndexProfileID !== undefined ? Boolean(activeIndexProfileID) : base.rag_enabled);
    startPending("knowledge");
    const payload: KnowledgeBaseUpdatePayload = {
      knowledge_base_id: base.id,
      category_id: changes.categoryID ?? base.category_id ?? "",
      name: base.name,
      description: base.description || "",
      rag_enabled: ragEnabled,
      active_index_profile_id: activeIndexProfileID,
    };
    const sent = sendEnvelope("knowledge_base_update", {
      request_id: newRequestID(),
      payload,
    });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const deleteKnowledgeBase = useCallback((knowledgeBaseID: string) => {
    if (!clientToken || !knowledgeBaseID) return false;
    startPending("knowledge");
    const payload: KnowledgeBaseDeletePayload = { knowledge_base_id: knowledgeBaseID };
    const sent = sendEnvelope("knowledge_base_delete", {
      request_id: newRequestID(),
      payload,
    });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const uploadDocument = useCallback(async () => {
    if (!activeKnowledgeBase?.id) return;
    startPending("knowledge");
    try {
      const picked = await DocumentPicker.getDocumentAsync({ copyToCacheDirectory: true, multiple: false, base64: false });
      if (picked.canceled || !picked.assets?.[0]) {
        stopPending("knowledge");
        return;
      }
      const uploaded = await uploadMobileAsset({ asset: picked.assets[0], baseURL: assetBaseURL, sessionID });
      const payload: KnowledgeDocumentIngestPayload = {
        knowledge_base_id: activeKnowledgeBase.id,
        url: uploaded.short_url,
        code: uploaded.code,
      };
      if (!sendEnvelope("knowledge_document_ingest", {
        request_id: newRequestID(),
        payload,
      })) {
        throw new Error("Document ingest request could not be sent");
      }
    } catch (error) {
      onError(error instanceof Error ? error.message : "Document upload failed");
      stopPending("knowledge");
    }
  }, [activeKnowledgeBase?.id, assetBaseURL, onError, sendEnvelope, sessionID, startPending, stopPending]);

  const retryDocument = useCallback((knowledgeBaseID: string, jobID: string) => {
    startPending("knowledge");
    const payload: KnowledgeDocumentRetryPayload = { knowledge_base_id: knowledgeBaseID, job_id: jobID };
    if (!sendEnvelope("knowledge_document_retry", { request_id: newRequestID(), payload })) {
      stopPending("knowledge");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const deleteDocument = useCallback((knowledgeBaseID: string, documentID: string) => {
    startPending("knowledge");
    const payload: KnowledgeDocumentDeletePayload = { knowledge_base_id: knowledgeBaseID, document_id: documentID };
    if (!sendEnvelope("knowledge_document_delete", { request_id: newRequestID(), payload })) {
      stopPending("knowledge");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const searchKnowledge = useCallback((query: string, rag?: RAGSettings) => {
    if (!clientToken || !query.trim()) return false;
    startPending("knowledge");
    const payload: KnowledgeSearchPreviewPayload = {
      query: query.trim(),
      knowledge_base_ids: rag?.knowledge_base_ids || [],
      category_ids: rag?.category_ids || [],
      top_k: rag?.top_k || 8,
    };
    const sent = sendEnvelope("knowledge_search_preview", {
      request_id: newRequestID(),
      payload,
    });
    if (!sent) stopPending("knowledge");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const setRAGSettings = useCallback((settings: RAGSettings) => {
    if (!clientToken) {
      onError("请先完成配对");
      return false;
    }
    if (!sessionID) {
      onError("请先加载会话");
      return false;
    }
    startPending("settings");
    const payload: SessionRAGSetPayload = { session_id: sessionID, ...settings };
    const sent = sendEnvelope("session_rag_set", { request_id: newRequestID(), session_id: sessionID, payload });
    if (!sent) stopPending("settings");
    return sent;
  }, [clientToken, onError, sendEnvelope, sessionID, startPending, stopPending]);

  return {
    createCategory,
    createKnowledgeBase,
    deleteCategory,
    deleteDocument,
    deleteKnowledgeBase,
    moveCategory,
    requestCatalog,
    requestDocuments,
    requestProfiles,
    retryDocument,
    searchKnowledge,
    setRAGSettings,
    updateKnowledgeBase,
    uploadDocument,
  };
}
