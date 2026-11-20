import { useCallback, useEffect, useRef, useState, type RefObject } from "react";

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
  KnowledgeDocumentListResultPayload,
  KnowledgeDocumentRetryPayload,
  KnowledgeSearchPreviewPayload,
  RelayMessage,
  RAGSettings,
  SessionRAGSetPayload,
} from "../protocol";
import type { PendingAction } from "../types/app";
import type { DocumentImportStatus, KnowledgeBaseChanges } from "../types/knowledge";
import { newRequestID } from "../utils/ids";
import * as DocumentPicker from "expo-document-picker";
import {
  AssetUploadCanceledError,
  AssetUploadDisconnectedError,
  assertAssetUploadScope,
  type RelayUploadScope,
  type UploadTransportErrorPayload,
} from "../utils/assetUpload";
import type { UploadMobileAsset } from "./useAssetUpload";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

const ingestRequestPrefix = "knowledge-ingest-";
const reviewRequestPrefix = "knowledge-import-list-";
const ingestTimeoutMs = 330000;
const reviewTimeoutMs = 30000;
const unknownImportMessage = "未能确认导入结果。请检查原知识库的文档列表，再决定是否重新上传。";

type DocumentImport = {
  scope: RelayUploadScope;
  sessionID: string;
  controller: AbortController;
  knowledgeBaseID: string;
  knowledgeBaseName: string;
  status?: DocumentImportStatus;
  ingestRequestID?: string;
  listing?: { requestID: string; connectionID: string };
  timeout?: ReturnType<typeof setTimeout>;
};

function clearImportTimeout(operation: DocumentImport) {
  clearTimeout(operation.timeout);
  operation.timeout = undefined;
}

type Args = {
  activeKnowledgeBase?: KnowledgeBase;
  applyDocuments: (payload?: KnowledgeDocumentListResultPayload) => void;
  uploadMobileAsset: UploadMobileAsset;
  clientToken: string;
  pairingID: string;
  relayScopeRef: RefObject<RelayUploadScope>;
  sessionID: string;
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
  onError: (message: string) => void;
};

export function useKnowledgeActions({
  activeKnowledgeBase,
  applyDocuments,
  uploadMobileAsset,
  clientToken,
  pairingID,
  relayScopeRef,
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

  const documentImportRef = useRef<DocumentImport | null>(null);
  const [uploadingDocument, setUploadingDocument] = useState(false);
  const [documentImportStatus, setDocumentImportStatus] = useState<DocumentImportStatus | null>(null);
  useEffect(() => () => {
    const operation = documentImportRef.current;
    documentImportRef.current = null;
    if (operation) {
      clearImportTimeout(operation);
      operation.controller.abort();
    }
  }, []);

  const updateImportStatus = useCallback((operation: DocumentImport, status: DocumentImportStatus) => {
    operation.status = status;
    setDocumentImportStatus(status);
  }, []);
  const cancelDocumentUpload = useCallback(() => {
    const operation = documentImportRef.current;
    if (!operation || operation.status) return;
    documentImportRef.current = null;
    operation.controller.abort();
    setUploadingDocument(false);
  }, []);
  const markImportUnknown = useCallback((operation: DocumentImport, message: string) => {
    if (!operation.status) return;
    clearImportTimeout(operation);
    operation.listing = undefined;
    updateImportStatus(operation, { ...operation.status, phase: "unknown", message });
  }, [updateImportStatus]);

  const reconcileDocumentImport = useCallback(() => {
    const operation = documentImportRef.current;
    const scope = { ...relayScopeRef.current };
    if (!operation?.status || !scope.connectionID || operation.scope.pairingID !== scope.pairingID ||
        (operation.status.phase !== "unknown" && operation.status.phase !== "review")) return false;
    const requestID = `${reviewRequestPrefix}${newRequestID()}`;
    operation.listing = { requestID, connectionID: scope.connectionID };
    updateImportStatus(operation, { ...operation.status, phase: "checking" });
    operation.timeout = setTimeout(() => {
      if (documentImportRef.current !== operation || operation.listing?.requestID !== requestID ||
          relayScopeRef.current.pairingID !== scope.pairingID) return;
      markImportUnknown(operation, "读取文档列表超时。请刷新原知识库后再决定是否重新上传。");
    }, reviewTimeoutMs);
    const payload: KnowledgeDocumentListPayload = { knowledge_base_id: operation.knowledgeBaseID };
    if (!sendEnvelope("knowledge_document_list", { request_id: requestID, session_id: operation.sessionID, payload })) {
      markImportUnknown(operation, "无法刷新文档列表，请连接后检查原知识库。");
      return false;
    }
    return true;
  }, [markImportUnknown, relayScopeRef, sendEnvelope, updateImportStatus]);

  const dismissDocumentImport = useCallback(() => {
    const operation = documentImportRef.current;
    if (!operation?.status || operation.scope.pairingID !== relayScopeRef.current.pairingID ||
        operation.status.phase === "importing" || operation.status.phase === "checking") return;
    clearImportTimeout(operation);
    documentImportRef.current = null;
    setDocumentImportStatus(null);
  }, [relayScopeRef]);

  const disconnectKnowledgeUploads = useCallback(() => {
    const operation = documentImportRef.current;
    if (!operation || operation.scope.pairingID !== relayScopeRef.current.pairingID) return;
    if (!operation.status) {
      cancelDocumentUpload();
      return;
    }
    if (operation.status.phase === "importing" || operation.status.phase === "checking") {
      markImportUnknown(operation, unknownImportMessage);
    }
  }, [cancelDocumentUpload, markImportUnknown, relayScopeRef]);

  const reconnectKnowledgeUploads = useCallback(() => {
    if (documentImportRef.current?.status?.phase === "unknown") reconcileDocumentImport();
  }, [reconcileDocumentImport]);

  const handleKnowledgeUploadMessage = useCallback((message: RelayMessage) => {
    const requestID = message.request_id || "";
    const isIngest = requestID.startsWith(ingestRequestPrefix);
    const isListing = requestID.startsWith(reviewRequestPrefix);
    if (!isIngest && !isListing) return false;
    const operation = documentImportRef.current;
    const scope = relayScopeRef.current;
    if (!operation?.status || scope.pairingID !== operation.scope.pairingID || !scope.connectionID ||
        (message.session_id && message.session_id !== operation.sessionID)) return true;
    if (isIngest ? operation.ingestRequestID !== requestID : (
      operation.listing?.requestID !== requestID || scope.connectionID !== operation.listing.connectionID
    )) return true;

    if (message.type === "error") {
      const error = (message.payload || {}) as UploadTransportErrorPayload;
      if (error.request_type && error.request_type !== (isIngest ? "knowledge_document_ingest" : "knowledge_document_list")) return true;
      if (!isIngest) {
        markImportUnknown(operation, error.message || "读取文档列表失败。请检查原知识库后再决定是否重新上传。");
      } else {
        // An Agent error can occur *after* saving a document (e.g. indexing
        // submission or the response's list failed), so also refresh on error.
        operation.ingestRequestID = undefined;
        markImportUnknown(operation, error.transport_error ? unknownImportMessage :
          `${error.message || "知识库导入失败。"} 请检查文档列表后再决定是否重新上传。`);
        if (!error.transport_error) reconcileDocumentImport();
      }
      return true;
    }

    const expectedType = isIngest ? "knowledge_document_mutation_result" : "knowledge_document_list_result";
    if (message.type !== expectedType) return true;
    const payload = message.payload as KnowledgeDocumentListResultPayload | undefined;
    if (payload?.knowledge_base_id !== operation.knowledgeBaseID || !Array.isArray(payload.documents)) return true;
    clearImportTimeout(operation);
    operation.listing = undefined;
    applyDocuments(payload);
    // A same-pair reconnect can recover a late *matching* ingest reply. A
    // listing alone has no asset ID and cannot prove that a name match is ours.
    if (isIngest) {
      operation.ingestRequestID = undefined;
      updateImportStatus(operation, { ...operation.status, phase: "complete", message: "文档已提交索引。" });
    } else {
      updateImportStatus(operation, { ...operation.status, phase: "review" });
    }
    return true;
  }, [applyDocuments, markImportUnknown, reconcileDocumentImport, relayScopeRef, updateImportStatus]);

  const uploadDocument = useCallback(async (knowledgeBaseID: string, knowledgeBaseName = knowledgeBaseID) => {
    const scope = { ...relayScopeRef.current };
    if (scope.pairingID !== pairingID || !knowledgeBaseID) return;
    const previous = documentImportRef.current;
    if (previous && previous.status?.phase !== "complete") return;
    if (!sessionID) {
      onError("请先加载会话再上传文件。");
      return;
    }
    if (!scope.connectionID) {
      onError("当前未连接 Relay，请先连接再上传文件。");
      return;
    }
    const controller = new AbortController();
    const operation: DocumentImport = { scope, sessionID, controller, knowledgeBaseID, knowledgeBaseName };
    documentImportRef.current = operation;
    setDocumentImportStatus(null);
    setUploadingDocument(true);
    try {
      const picked = await DocumentPicker.getDocumentAsync({ copyToCacheDirectory: true, multiple: false, base64: false });
      if (documentImportRef.current !== operation || controller.signal.aborted) return;
      assertAssetUploadScope(scope, relayScopeRef.current);
      if (picked.canceled || !picked.assets?.[0]) return;
      const asset = picked.assets[0];
      const uploaded = await uploadMobileAsset({ asset, scope, sessionID, signal: controller.signal });
      if (documentImportRef.current !== operation || controller.signal.aborted) return;
      assertAssetUploadScope(scope, relayScopeRef.current);
      const requestID = `${ingestRequestPrefix}${newRequestID()}`;
      operation.ingestRequestID = requestID;
      setUploadingDocument(false);
      updateImportStatus(operation, {
        phase: "importing",
        knowledgeBaseID,
        knowledgeBaseName,
        fileName: uploaded.file_name || asset.name,
        message: "正在将文档加入知识库，请等待确认。",
      });
      operation.timeout = setTimeout(() => {
        if (documentImportRef.current !== operation || operation.status?.phase !== "importing" ||
            relayScopeRef.current.pairingID !== pairingID) return;
        markImportUnknown(operation, unknownImportMessage);
        reconcileDocumentImport();
      }, ingestTimeoutMs);
      const payload: KnowledgeDocumentIngestPayload = { knowledge_base_id: knowledgeBaseID, asset_id: uploaded.asset_id };
      if (!sendEnvelope("knowledge_document_ingest", { request_id: requestID, session_id: sessionID, payload })) {
        operation.ingestRequestID = undefined;
        markImportUnknown(operation, "未能提交导入请求，请检查连接和原知识库的文档列表。");
      }
    } catch (error) {
      if (documentImportRef.current === operation && relayScopeRef.current.pairingID === pairingID && !(error instanceof AssetUploadCanceledError)) {
        const failure = relayScopeRef.current.connectionID === scope.connectionID ? error : new AssetUploadDisconnectedError();
        onError(failure instanceof Error ? failure.message : "文档上传失败。");
      }
    } finally {
      if (documentImportRef.current === operation && !operation.status) {
        documentImportRef.current = null;
        setUploadingDocument(false);
      }
    }
  }, [markImportUnknown, onError, pairingID, reconcileDocumentImport, relayScopeRef, sendEnvelope, sessionID, updateImportStatus, uploadMobileAsset]);

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
    cancelDocumentUpload,
    disconnectKnowledgeUploads,
    dismissDocumentImport,
    documentImportStatus,
    handleKnowledgeUploadMessage,
    reconcileDocumentImport,
    reconnectKnowledgeUploads,
    uploadingDocument,
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
