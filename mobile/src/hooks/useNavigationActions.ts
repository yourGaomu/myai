import { useCallback } from "react";

import type { ViewMode } from "../types/app";

type Args = {
  fileEntriesCount: number;
  filePath: string;
  loadSession: (sessionID: string) => void;
  requestAssets: () => boolean;
  requestChanges: () => boolean;
  requestFiles: (path?: string) => boolean;
  requestSessions: () => boolean;
  setViewMode: (updater: ViewMode | ((current: ViewMode) => ViewMode)) => void;
};

export function useNavigationActions({
  fileEntriesCount,
  filePath,
  loadSession,
  requestAssets,
  requestChanges,
  requestFiles,
  requestSessions,
  setViewMode,
}: Args) {
  const openChat = useCallback(() => setViewMode("chat"), [setViewMode]);
  const openChanges = useCallback(() => {
    setViewMode("changes");
    requestChanges();
  }, [requestChanges, setViewMode]);
  const openFiles = useCallback(() => {
    setViewMode("files");
    requestAssets();
    if (fileEntriesCount === 0) {
      requestFiles(filePath);
    }
  }, [fileEntriesCount, filePath, requestAssets, requestFiles, setViewMode]);
  const openSessions = useCallback(() => {
    setViewMode("sessions");
    requestSessions();
  }, [requestSessions, setViewMode]);
  const toggleSettings = useCallback(() => {
    setViewMode((value) => (value === "settings" ? "chat" : "settings"));
  }, [setViewMode]);
  const selectSession = useCallback(
    (nextSessionID: string) => {
      loadSession(nextSessionID);
      setViewMode("chat");
    },
    [loadSession, setViewMode],
  );

  return {
    openChanges,
    openChat,
    openFiles,
    openSessions,
    selectSession,
    toggleSettings,
  };
}
