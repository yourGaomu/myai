import { useCallback, useState } from "react";

import type {
  KnowledgeBase,
  KnowledgeCatalogResultPayload,
  KnowledgeDocument,
  KnowledgeDocumentListResultPayload,
  KnowledgeIndexProfile,
  KnowledgeIndexingJob,
  KnowledgeProfileListResultPayload,
  KnowledgeSearchPreviewResultPayload,
  RAGSettings,
} from "../protocol";

export function useKnowledgeState() {
  const [categories, setCategories] = useState<KnowledgeCatalogResultPayload["categories"]>([]);
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
  const [documentsByBase, setDocumentsByBase] = useState<Record<string, KnowledgeDocument[]>>({});
  const [jobsByBase, setJobsByBase] = useState<Record<string, KnowledgeIndexingJob[]>>({});
  const [profiles, setProfiles] = useState<KnowledgeIndexProfile[]>([]);
  const [searchResult, setSearchResult] = useState<KnowledgeSearchPreviewResultPayload | null>(null);
  const [selectedKnowledgeBaseID, setSelectedKnowledgeBaseID] = useState("");
  const [message, setMessage] = useState("");

  const applyCatalog = useCallback((payload?: KnowledgeCatalogResultPayload) => {
    if (!payload) return;
    setCategories(payload.categories || []);
    setKnowledgeBases(payload.knowledge_bases || []);
    setSelectedKnowledgeBaseID((current) => current && !(payload.knowledge_bases || []).some((base) => base.id === current) ? "" : current);
    setMessage(payload.message || "");
  }, []);

  const applyDocuments = useCallback((payload?: KnowledgeDocumentListResultPayload) => {
    if (!payload?.knowledge_base_id) return;
    setDocumentsByBase((current) => ({ ...current, [payload.knowledge_base_id]: payload.documents || [] }));
    setJobsByBase((current) => ({ ...current, [payload.knowledge_base_id]: payload.jobs || [] }));
    setMessage(payload.message || "");
  }, []);

  const applyProfiles = useCallback((payload?: KnowledgeProfileListResultPayload) => {
    setProfiles(payload?.profiles || []);
  }, []);

  const applySearchResult = useCallback((payload?: KnowledgeSearchPreviewResultPayload) => {
    setSearchResult(payload || null);
  }, []);

  const clearKnowledge = useCallback(() => {
    setCategories([]);
    setKnowledgeBases([]);
    setDocumentsByBase({});
    setJobsByBase({});
    setProfiles([]);
    setSearchResult(null);
    setMessage("");
  }, []);

  const activeRAGSettings = (sessionRAG?: RAGSettings): RAGSettings => ({
    mode: sessionRAG?.mode || "auto",
    knowledge_base_ids: sessionRAG?.knowledge_base_ids || [],
    category_ids: sessionRAG?.category_ids || [],
    top_k: sessionRAG?.top_k || 8,
  });

  return {
    activeRAGSettings,
    applyCatalog,
    applyDocuments,
    applyProfiles,
    applySearchResult,
    categories,
    clearKnowledge,
    documentsByBase,
    jobsByBase,
    knowledgeBases,
    message,
    profiles,
    searchResult,
    selectedKnowledgeBaseID,
    setMessage,
    setSelectedKnowledgeBaseID,
  };
}
