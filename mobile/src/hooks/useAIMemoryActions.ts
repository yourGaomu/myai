import { useCallback } from "react";

import type {
  AIMemoryCandidate,
  AIMemoryCandidateApprovePayload,
  AIMemoryCandidateRejectPayload,
  AIMemoryCreatePayload,
  AIMemoryDeletePayload,
  AIMemoryInput,
  AIMemoryListPayload,
  AIMemoryRestorePayload,
  AIMemoryUpdatePayload,
  RelayMessage,
} from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  clientToken: string;
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useAIMemoryActions({ clientToken, sendEnvelope, startPending, stopPending }: Args) {
  const send = useCallback((type: RelayMessage["type"], payload: unknown) => {
    if (!clientToken) return false;
    startPending("memory");
    const sent = sendEnvelope(type, { request_id: newRequestID(), payload });
    if (!sent) stopPending("memory");
    return sent;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const requestMemories = useCallback((filter: AIMemoryListPayload = {}) => send("ai_memory_list", filter), [send]);
  const requestCandidates = useCallback(() => send("ai_memory_candidate_list", { statuses: ["pending"], limit: 100 }), [send]);

  const createMemory = useCallback((memory: AIMemoryInput) => {
    const payload: AIMemoryCreatePayload = { memory };
    return send("ai_memory_create", payload);
  }, [send]);

  const updateMemory = useCallback((memoryID: string, memory: AIMemoryInput) => {
    const payload: AIMemoryUpdatePayload = { memory_id: memoryID, memory };
    return send("ai_memory_update", payload);
  }, [send]);

  const deleteMemory = useCallback((memoryID: string) => {
    const payload: AIMemoryDeletePayload = { memory_id: memoryID, reason: "deleted by user" };
    return send("ai_memory_delete", payload);
  }, [send]);

  const restoreMemory = useCallback((memoryID: string) => {
    const payload: AIMemoryRestorePayload = { memory_id: memoryID };
    return send("ai_memory_restore", payload);
  }, [send]);

  const approveCandidate = useCallback((candidate: AIMemoryCandidate, memoryID = "") => {
    const payload: AIMemoryCandidateApprovePayload = { candidate_id: candidate.id, memory_id: memoryID || undefined };
    return send("ai_memory_candidate_approve", payload);
  }, [send]);

  const rejectCandidate = useCallback((candidate: AIMemoryCandidate, note = "rejected by user") => {
    const payload: AIMemoryCandidateRejectPayload = { candidate_id: candidate.id, note };
    return send("ai_memory_candidate_reject", payload);
  }, [send]);

  return {
    approveCandidate,
    createMemory,
    deleteMemory,
    rejectCandidate,
    requestCandidates,
    requestMemories,
    restoreMemory,
    updateMemory,
  };
}
