import { useCallback } from "react";
import { Alert } from "react-native";

import type { ChangeDiffResultPayload, ChangeEntry, HistoryCheckpoint, RelayMessage } from "../protocol";
import type { PendingAction } from "../types/app";
import { formatDateTime } from "../utils/format";
import { newRequestID, shortID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  canRevertSelectedChange: boolean;
  changeDiff: ChangeDiffResultPayload | null;
  historyCheckpoints: HistoryCheckpoint[];
  selectedChange: string;
  sendEnvelope: SendEnvelope;
  setChangeDiff: (diff: ChangeDiffResultPayload | null) => void;
  setHistoryDiff: (diff: null) => void;
  setSelectedChange: (path: string) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 文件恢复是破坏性操作，必须先弹确认框，再发送 baseline 或 checkpoint 恢复命令。
export function useChangeHistoryActions({
  canRevertSelectedChange,
  changeDiff,
  historyCheckpoints,
  selectedChange,
  sendEnvelope,
  setChangeDiff,
  setHistoryDiff,
  setSelectedChange,
  startPending,
  stopPending,
}: Args) {
  const openChangeEntry = useCallback(
    (entry: ChangeEntry) => {
      setSelectedChange(entry.path);
      setChangeDiff(null);
      setHistoryDiff(null);
      startPending("diff");
      if (!sendEnvelope("change_diff", {
        request_id: newRequestID(),
        payload: { path: entry.path },
      })) {
        stopPending("diff");
      }
    },
    [sendEnvelope, setChangeDiff, setHistoryDiff, setSelectedChange, startPending, stopPending],
  );

  const revertSelectedChange = useCallback(() => {
    const path = changeDiff?.path || selectedChange;
    if (!path || !canRevertSelectedChange) {
      return;
    }

    Alert.alert("Revert file change?", `Restore ${path} to the saved baseline.`, [
      { text: "Cancel", style: "cancel" },
      {
        text: "Revert",
        style: "destructive",
        onPress: () => {
          startPending("revert");
          if (!sendEnvelope("change_revert", {
            request_id: newRequestID(),
            payload: { path },
          })) {
            stopPending("revert");
          }
        },
      },
    ]);
  }, [canRevertSelectedChange, changeDiff?.path, selectedChange, sendEnvelope, startPending, stopPending]);

  const revertHistoryCheckpoint = useCallback(
    (checkpointID: string) => {
      if (!checkpointID) {
        return;
      }

      const checkpoint = historyCheckpoints.find((item) => item.id === checkpointID);
      const title = checkpoint?.title || `Checkpoint ${shortID(checkpointID)}`;
      const detail = checkpoint
        ? `${checkpoint.change_count} file(s) from ${formatDateTime(checkpoint.created_at)} will be restored.`
        : "This checkpoint will be restored.";

      Alert.alert("Revert checkpoint?", `${title}\n${detail}`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Revert",
          style: "destructive",
          onPress: () => {
            startPending("revert");
            if (!sendEnvelope("history_revert", {
              request_id: newRequestID(),
              payload: { checkpoint_id: checkpointID },
            })) {
              stopPending("revert");
            }
          },
        },
      ]);
    },
    [historyCheckpoints, sendEnvelope, startPending, stopPending],
  );

  const previewHistoryCheckpoint = useCallback(
    (checkpointID: string) => {
      if (!checkpointID) {
        return;
      }

      setSelectedChange("");
      setChangeDiff(null);
      startPending("diff");
      if (!sendEnvelope("history_diff", {
        request_id: newRequestID(),
        payload: { checkpoint_id: checkpointID },
      })) {
        stopPending("diff");
      }
    },
    [sendEnvelope, setChangeDiff, setSelectedChange, startPending, stopPending],
  );

  return {
    openChangeEntry,
    previewHistoryCheckpoint,
    revertHistoryCheckpoint,
    revertSelectedChange,
  };
}
