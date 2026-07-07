import { useMemo } from "react";

import type {
  ChangeDiffResultPayload,
  ChangeEntry,
  FileReadResultPayload,
  ModelSummary,
  SessionSummary,
} from "../protocol";
import type { ViewMode } from "../types/app";
import type { ChatAttachment } from "../types/chat";
import { isWorkspaceFileAttachment } from "../utils/attachments";

type Args = {
  attachedFiles: ChatAttachment[];
  changeDiff: ChangeDiffResultPayload | null;
  changes: ChangeEntry[];
  currentModelID: string;
  filePreview: FileReadResultPayload | null;
  models: ModelSummary[];
  selectedChange: string;
  sessionID: string;
  sessions: SessionSummary[];
  viewMode: ViewMode;
};

export function useMobileDerivedState({
  attachedFiles,
  changeDiff,
  changes,
  currentModelID,
  filePreview,
  models,
  selectedChange,
  sessionID,
  sessions,
  viewMode,
}: Args) {
  const activeSession = useMemo(
    () => sessions.find((session) => session.id === sessionID),
    [sessionID, sessions],
  );
  const activeModel = useMemo(
    () => models.find((model) => model.id === currentModelID),
    [currentModelID, models],
  );
  const selectedChangeEntry = useMemo(
    () => changes.find((entry) => entry.path === selectedChange),
    [changes, selectedChange],
  );

  return {
    activeModel,
    activeSession,
    canOpenSelectedChangeFile: Boolean(selectedChange) && !selectedChangeEntry?.deleted,
    canRevertSelectedChange: Boolean(changeDiff?.restorable || selectedChangeEntry?.restorable),
    changesTabActive: viewMode === "changes" || viewMode === "changeDetail",
    filePreviewAttached: Boolean(
      filePreview && attachedFiles.some((file) => isWorkspaceFileAttachment(file) && file.path === filePreview.path),
    ),
    selectedChangeEntry,
    setupVisible: viewMode === "settings",
  };
}
