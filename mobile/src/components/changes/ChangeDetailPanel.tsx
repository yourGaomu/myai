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
          <Text style={styles.detailBackText}>❮ 返回变更</Text>
        </Pressable>
        <View style={styles.detailHeaderCenter}>
          <Text numberOfLines={1} style={styles.detailHeaderTitle}>
            {changeDiff
              ? changeDiff.path.split("/").pop() || changeDiff.path
              : historyDiff
                ? `检查点 ${shortID(historyDiff.checkpoint_id)}`
                : "差异详情"}
          </Text>
        </View>
        <View style={styles.rowCompact}>
          {changeDiff ? (
            <>
              <Pressable
                disabled={!canOpenSelectedChangeFile || pendingFiles}
                onPress={onOpenSelectedChangeFile}
                style={({ pressed }) =>
                  buttonFeedback([styles.previewButton, (!canOpenSelectedChangeFile || pendingFiles) && styles.disabledButton], pressed)
                }
              >
                <ButtonContent loading={pendingFiles} text={pendingFiles ? "打开中" : "打开"} />
              </Pressable>
              <Pressable
                disabled={!canRevertSelectedChange || pendingRevert}
                onPress={onRevertSelectedChange}
                style={({ pressed }) =>
                  buttonFeedback([styles.revertButton, (!canRevertSelectedChange || pendingRevert) && styles.disabledButton], pressed)
                }
              >
                <ButtonContent loading={pendingRevert} text={pendingRevert ? "还原中" : "还原文件"} />
              </Pressable>
            </>
          ) : historyDiff ? (
            <Pressable
              disabled={pendingRevert}
              onPress={() => onRevertHistory(historyDiff.checkpoint_id)}
              style={({ pressed }) => buttonFeedback([styles.revertButton, pendingRevert && styles.disabledButton], pressed)}
            >
              <ButtonContent loading={pendingRevert} text={pendingRevert ? "恢复中" : "恢复检查点"} />
            </Pressable>
          ) : null}
        </View>
      </View>

      {changeDiff ? (
        <View style={styles.diffDetailContent}>
          <View style={styles.diffStatsBar}>
            <Text numberOfLines={1} style={styles.diffStatsPath}>
              {changeDiff.path}
              {changeDiff.truncated ? " / 已截断" : ""}
            </Text>
          </View>
          {changeDiff.binary ? (
            <Text style={styles.emptyText}>{changeDiff.message || "二进制文件暂不支持差异查看。"}</Text>
          ) : (
            <DiffViewer diff={changeDiff.diff || ""} emptyText={changeDiff.message || "No diff is available."} />
          )}
        </View>
      ) : null}

      {historyDiff ? (
        <View style={styles.diffDetailContent}>
          <View style={styles.diffStatsBar}>
            <Text style={styles.diffStatsPath}>
              快照 ID: {historyDiff.checkpoint_id} ({(historyDiff.files || []).length} 个文件变动)
            </Text>
          </View>
          {historyDiff.message && (historyDiff.files || []).length > 0 ? <Text style={styles.emptyText}>{historyDiff.message}</Text> : null}
          {(historyDiff.files || []).length === 0 ? (
            <Text style={styles.emptyText}>{historyDiff.message || "没有可显示的差异。"}</Text>
          ) : (
            (historyDiff.files || []).map((file) => (
              <View key={`${historyDiff.checkpoint_id}-${file.path}`} style={styles.historyDiffFile}>
                <Text style={styles.fileName}>
                  {file.path}
                  {file.truncated ? " / 已截断" : ""}
                </Text>
                <Text style={styles.fileMeta}>{file.change_type || "已变更"}</Text>
                {file.binary ? (
                  <Text style={styles.emptyText}>{file.message || "二进制文件暂不支持差异查看。"}</Text>
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
