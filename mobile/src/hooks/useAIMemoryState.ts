import { useCallback, useState } from "react";

import type {
  AIMemory,
  AIMemoryCandidate,
  AIMemoryCandidateListResultPayload,
  AIMemoryCandidateMutationResultPayload,
  AIMemoryExtractionJob,
  AIMemoryExtractionJobListResultPayload,
  AIMemoryDreamRun,
  AIMemoryDreamResultPayload,
  AIMemoryListResultPayload,
} from "../protocol";

export function useAIMemoryState() {
  const [memories, setMemories] = useState<AIMemory[]>([]);
  const [candidates, setCandidates] = useState<AIMemoryCandidate[]>([]);
  const [extractionJobs, setExtractionJobs] = useState<AIMemoryExtractionJob[]>([]);
  const [dreamRuns, setDreamRuns] = useState<AIMemoryDreamRun[]>([]);
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

  const applyExtractionJobs = useCallback((payload?: AIMemoryExtractionJobListResultPayload) => {
    if (!payload) return;
    setExtractionJobs(payload.jobs || []);
    setMessage(payload.message || "");
  }, []);

  const applyDreamResult = useCallback((payload?: AIMemoryDreamResultPayload) => {
    if (!payload) return;
    setDreamRuns(payload.runs || []);
    if (payload.memories) setMemories(payload.memories);
    if (payload.candidates) setCandidates(payload.candidates);
    setMessage(payload.message || "");
  }, []);

  const clear = useCallback(() => {
    setMemories([]);
    setCandidates([]);
    setExtractionJobs([]);
    setDreamRuns([]);
    setMessage("");
  }, []);

  return {
    applyCandidateMutation,
    applyCandidates,
    applyExtractionJobs,
    applyDreamResult,
    applyMemories,
    candidates,
    clear,
    extractionJobs,
    dreamRuns,
    memories,
    message,
    setMessage,
  };
}
