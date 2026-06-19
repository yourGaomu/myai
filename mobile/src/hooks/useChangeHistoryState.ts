import { useCallback, useState } from "react";

import type { ChangeDiffResultPayload, ChangeEntry, HistoryCheckpoint, HistoryDiffResultPayload } from "../protocol";

export function useChangeHistoryState() {
  const [changes, setChanges] = useState<ChangeEntry[]>([]);
  const [changesMessage, setChangesMessage] = useState("");
  const [changesClean, setChangesClean] = useState(false);
  const [selectedChange, setSelectedChange] = useState("");
  const [changeDiff, setChangeDiff] = useState<ChangeDiffResultPayload | null>(null);
  const [historyCheckpoints, setHistoryCheckpoints] = useState<HistoryCheckpoint[]>([]);
  const [historyDiff, setHistoryDiff] = useState<HistoryDiffResultPayload | null>(null);
  const [historyMessage, setHistoryMessage] = useState("");

  const clearWorkspaceChanges = useCallback(() => {
    setChanges([]);
    setChangesMessage("");
    setChangesClean(false);
    setChangeDiff(null);
  }, []);

  const clearHistory = useCallback(() => {
    setHistoryCheckpoints([]);
    setHistoryDiff(null);
    setHistoryMessage("");
  }, []);

  return {
    changeDiff,
    changes,
    changesClean,
    changesMessage,
    clearHistory,
    clearWorkspaceChanges,
    historyCheckpoints,
    historyDiff,
    historyMessage,
    selectedChange,
    setChangeDiff,
    setChanges,
    setChangesClean,
    setChangesMessage,
    setHistoryCheckpoints,
    setHistoryDiff,
    setHistoryMessage,
    setSelectedChange,
  };
}
