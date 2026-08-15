import { useCallback, useState } from "react";

import type {
  AIMemory,
  AIMemoryCandidate,
  AIMemoryCandidateListResultPayload,
  AIMemoryCandidateMutationResultPayload,
  AIMemoryListResultPayload,
} from "../protocol";

export function useAIMemoryState() {
  const [memories, setMemories] = useState<AIMemory[]>([]);
  const [candidates, setCandidates] = useState<AIMemoryCandidate[]>([]);
  const [message, setMessage] = useState("");

  const applyMemories = useCallback((payload?: AIMemoryListResultPayload) => {
    if (!payload) return;
    setMemories(payload.memories || []);
    setMessage(payload.message || "");
  }, []);

  const applyCandidates = useCallback((payload?: AIMemoryCandidateListResultPayload) => {
    if (!payload) return;
    setCandidates(payload.candidates || []);
    setMessage(payload.message || "");
  }, []);

  const applyCandidateMutation = useCallback((payload?: AIMemoryCandidateMutationResultPayload) => {
    if (!payload) return;
    setMemories(payload.memories || []);
    setCandidates(payload.candidates || []);
    setMessage(payload.message || "");
  }, []);

  const clear = useCallback(() => {
    setMemories([]);
    setCandidates([]);
    setMessage("");
  }, []);

  return {
    applyCandidateMutation,
    applyCandidates,
    applyMemories,
    candidates,
    clear,
    memories,
    message,
    setMessage,
  };
}
