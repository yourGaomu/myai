import { Pressable, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { ChangeEntry, HistoryCheckpoint } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { changeLabel, changeMeta } from "../../utils/changes";
import { formatDateTime } from "../../utils/format";
import { shortID } from "../../utils/ids";
import { changeBadgeStyle } from "./changeUtils";
import { styles } from "./styles";

type Props = {
  buttonFeedback: ButtonFeedback;
  changes: ChangeEntry[];
  changesClean: boolean;
  changesMessage: string;
  clientToken: string;
  historyCheckpoints: HistoryCheckpoint[];
  historyMessage: string;
  onOpenChange: (entry: ChangeEntry) => void;
  onPreviewHistory: (checkpointID: string) => void;
  onRefreshChanges: () => void;
  onRefreshHistory: () => void;
  onRevertHistory: (checkpointID: string) => void;
  pendingChanges: boolean;
  pendingDiff: boolean;
  pendingHistory: boolean;
  pendingRevert: boolean;
  selectedChange: string;
};

export function ChangesPanel({
  buttonFeedback,
  changes,
  changesClean,
  changesMessage,
  clientToken,
  historyCheckpoints,
  historyMessage,
  onOpenChange,
  onPreviewHistory,
  onRefreshChanges,
  onRefreshHistory,
  onRevertHistory,
  pendingChanges,
  pendingDiff,
  pendingHistory,
  pendingRevert,
  selectedChange,
}: Props) {
  return (
    <View style={styles.container}>
      {/* 变更文件列表 */}
      <View style={[styles.panel, styles.changesPanel]}>
        <View style={styles.panelHeader}>
          <View style={styles.flex}>
            <Text style={styles.panelTitle}>工作区文件变更</Text>
            <Text style={styles.pathText}>{changesClean ? "工作区干净" : `${changes.length} 个文件变动`}</Text>
          </View>
          <View style={styles.rowCompact}>
            <Pressable
              disabled={pendingChanges}
              onPress={onRefreshChanges}
              style={({ pressed }) => buttonFeedback([styles.smallButton, pendingChanges && styles.disabledButton], pressed)}
            >
              <ButtonContent loading={pendingChanges} text={pendingChanges ? "加载中" : "刷新"} />
            </Pressable>
          </View>
        </View>

        <View style={styles.changeList}>
          {changesMessage ? <Text style={styles.emptyText}>{changesMessage}</Text> : null}
          {!changesMessage && changes.length === 0 ? (
            <Text style={styles.emptyText}>{clientToken ? "当前工作区暂无未提交变更" : "请先完成配对"}</Text>
          ) : (
            changes.map((entry) => (
              <Pressable
                key={`${entry.path}-${entry.index_status || ""}-${entry.worktree_status || ""}`}
                disabled={pendingDiff}
                onPress={() => onOpenChange(entry)}
                style={({ pressed }) =>
                  buttonFeedback([styles.changeRow, selectedChange === entry.path && styles.changeRowActive, pendingDiff && styles.disabledButton], pressed)
                }
              >
                <Text style={[styles.changeBadge, changeBadgeStyle(entry)]}>{changeLabel(entry)}</Text>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.fileName}>{entry.path}</Text>
                  <Text style={styles.fileMeta}>{changeMeta(entry)}</Text>
                </View>
                <Text style={styles.viewDiffText}>查看 Diff ❯</Text>
              </Pressable>
            ))
          )}
        </View>
      </View>

      {/* 历史快照检查点 */}
      <View style={styles.historyBox}>
        <View style={styles.previewHeader}>
          <View style={styles.flex}>
            <Text style={styles.previewTitle}>历史版本快照</Text>
            <Text style={styles.pathText}>SQLite Checkpoints 检查点</Text>
          </View>
          <Pressable
            disabled={pendingHistory}
            onPress={onRefreshHistory}
            style={({ pressed }) => buttonFeedback([styles.smallButton, pendingHistory && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingHistory} text={pendingHistory ? "加载中" : "刷新"} />
          </Pressable>
        </View>

        <View style={styles.historyList}>
          {historyMessage ? <Text style={styles.emptyText}>{historyMessage}</Text> : null}
          {!historyMessage && historyCheckpoints.length === 0 ? (
            <Text style={styles.emptyText}>{clientToken ? "还没有加载历史记录" : "请先完成配对"}</Text>
          ) : (
            historyCheckpoints.map((checkpoint) => (
              <View key={checkpoint.id} style={styles.historyRow}>
                <View style={styles.historyHeader}>
                  <Text style={styles.historyTag}>🏷️ {checkpoint.title || `Checkpoint ${shortID(checkpoint.id)}`}</Text>
                  <Text style={styles.historyCount}>{checkpoint.change_count} 文件</Text>
                </View>
                <Text style={styles.historyMeta}>{formatDateTime(checkpoint.created_at)}</Text>
                <View style={styles.historyActions}>
                  <Pressable
                    disabled={pendingDiff}
                    onPress={() => onPreviewHistory(checkpoint.id)}
                    style={({ pressed }) => buttonFeedback([styles.previewButton, pendingDiff && styles.disabledButton], pressed)}
                  >
                    <ButtonContent loading={pendingDiff} text={pendingDiff ? "加载中" : "对比"} />
                  </Pressable>
                  <Pressable
                    disabled={pendingRevert}
                    onPress={() => onRevertHistory(checkpoint.id)}
                    style={({ pressed }) => buttonFeedback([styles.revertButton, pendingRevert && styles.disabledButton], pressed)}
                  >
                    <ButtonContent loading={pendingRevert} text={pendingRevert ? "恢复中" : "一键回滚"} />
                  </Pressable>
                </View>
              </View>
            ))
          )}
        </View>
      </View>
    </View>
  );
}
