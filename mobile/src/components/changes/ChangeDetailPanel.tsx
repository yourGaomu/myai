import { Pressable, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { ChangeDiffResultPayload, HistoryDiffResultPayload } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { shortID } from "../../utils/ids";
import { DiffViewer } from "./DiffViewer";
import { styles } from "./styles";

type Props = {
  buttonFeedback: ButtonFeedback;
  canOpenSelectedChangeFile: boolean;
  canRevertSelectedChange: boolean;
  changeDiff: ChangeDiffResultPayload | null;
  historyDiff: HistoryDiffResultPayload | null;
  onBack: () => void;
  onOpenSelectedChangeFile: () => void;
  onRevertHistory: (checkpointID: string) => void;
  onRevertSelectedChange: () => void;
  pendingFiles: boolean;
  pendingRevert: boolean;
};

export function ChangeDetailPanel({
  buttonFeedback,
  canOpenSelectedChangeFile,
  canRevertSelectedChange,
  changeDiff,
  historyDiff,
  onBack,
  onOpenSelectedChangeFile,
  onRevertHistory,
  onRevertSelectedChange,
  pendingFiles,
  pendingRevert,
}: Props) {
  return (
    <View style={[styles.panel, styles.diffDetailPanel]}>
      <View style={styles.detailHeader}>
        <Pressable onPress={onBack} style={({ pressed }) => buttonFeedback(styles.detailBackButton, pressed)}>
          <Text style={styles.detailBackText}>Back</Text>
        </Pressable>
        <View style={styles.flex}>
          <Text style={styles.panelTitle}>{changeDiff ? "File Change" : "History Diff"}</Text>
          <Text numberOfLines={2} style={styles.pathText}>
            {changeDiff
              ? `${changeDiff.path}${changeDiff.truncated ? " / truncated" : ""}`
              : historyDiff
                ? `Checkpoint ${shortID(historyDiff.checkpoint_id)} / ${(historyDiff.files || []).length} file(s)`
                : "No diff selected"}
          </Text>
        </View>
      </View>

      {changeDiff ? (
        <View style={styles.diffDetailContent}>
          <View style={styles.detailActions}>
            <Pressable
              disabled={!canRevertSelectedChange || pendingRevert}
              onPress={onRevertSelectedChange}
              style={({ pressed }) =>
                buttonFeedback([styles.previewButton, (!canRevertSelectedChange || pendingRevert) && styles.disabledButton], pressed)
              }
            >
              <ButtonContent loading={pendingRevert} text={pendingRevert ? "Reverting" : "Revert"} />
            </Pressable>
            <Pressable
              disabled={!canOpenSelectedChangeFile || pendingFiles}
              onPress={onOpenSelectedChangeFile}
              style={({ pressed }) =>
                buttonFeedback([styles.previewButton, (!canOpenSelectedChangeFile || pendingFiles) && styles.disabledButton], pressed)
              }
            >
              <ButtonContent loading={pendingFiles} text={pendingFiles ? "Opening" : "Open file"} />
            </Pressable>
          </View>
          {changeDiff.binary ? (
            <Text style={styles.emptyText}>{changeDiff.message || "Binary diff is not available."}</Text>
          ) : (
            <DiffViewer diff={changeDiff.diff || ""} emptyText={changeDiff.message || "No diff is available."} />
          )}
        </View>
      ) : null}

      {historyDiff ? (
        <View style={styles.diffDetailContent}>
          <View style={styles.detailActions}>
            <Pressable
              disabled={pendingRevert}
              onPress={() => onRevertHistory(historyDiff.checkpoint_id)}
              style={({ pressed }) => buttonFeedback([styles.previewButton, pendingRevert && styles.disabledButton], pressed)}
            >
              <ButtonContent loading={pendingRevert} text={pendingRevert ? "Reverting" : "Revert checkpoint"} />
            </Pressable>
          </View>
          {historyDiff.message && (historyDiff.files || []).length > 0 ? <Text style={styles.emptyText}>{historyDiff.message}</Text> : null}
          {(historyDiff.files || []).length === 0 ? (
            <Text style={styles.emptyText}>{historyDiff.message || "No diff is available."}</Text>
          ) : (
            (historyDiff.files || []).map((file) => (
              <View key={`${historyDiff.checkpoint_id}-${file.path}`} style={styles.historyDiffFile}>
                <Text style={styles.fileName}>
                  {file.path}
                  {file.truncated ? " / truncated" : ""}
                </Text>
                <Text style={styles.fileMeta}>{file.change_type || "changed"}</Text>
                {file.binary ? (
                  <Text style={styles.emptyText}>{file.message || "Binary diff is not available."}</Text>
                ) : (
                  <DiffViewer diff={file.diff || ""} emptyText={file.message || "No diff is available."} />
                )}
              </View>
            ))
          )}
        </View>
      ) : null}
    </View>
  );
}
