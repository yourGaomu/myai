import { Fragment, useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ChangeEntry, HistoryCheckpoint } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { changeLabel } from "../../utils/changes";
import { formatDateTime, formatShortTime } from "../../utils/format";
import { shortID } from "../../utils/ids";
import { baseName, dirName } from "../../utils/toolDisplay";
import { fontFamily, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { ConfirmModal } from "../common/ConfirmModal";
import { useListStyles } from "../common/listStyles";
import { SkeletonRows } from "../common/MotionEffects";
import { Segmented } from "../common/Segmented";

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
  onRevertHistory: (checkpointID: string) => void;
  pendingChanges: boolean;
  pendingDiff: boolean;
  pendingHistory: boolean;
  pendingRevert: boolean;
  selectedChange: string;
};

// 变更页：展示未提交文件的 +X/-Y 行统计 + 历史快照回滚（带统一确认弹窗）。
export function ChangesPanel(props: Props) {
  const { palette } = useAppTheme();
  const list = useListStyles();
  const styles = useThemedStyles(makeStyles);
  const { changes, clientToken, historyCheckpoints, pendingDiff } = props;
  const [tab, setTab] = useState<"now" | "history">("now");
  const [confirmCheckpoint, setConfirmCheckpoint] = useState<HistoryCheckpoint | null>(null);

  function letterStyle(entry: ChangeEntry) {
    if (entry.deleted) return list.letterDel;
    if (entry.untracked || entry.status === "added") return list.letterAdd;
    if (entry.renamed) return list.letterRen;
    return list.letterMod;
  }

  return (
    <View>
      <Segmented onChange={setTab} options={[{ key: "now", label: "未提交" }, { key: "history", label: "历史快照" }]} value={tab} />

      {tab === "now" ? (
        <>
          <Text style={list.hint}>
            {props.changesMessage || (props.changesClean || changes.length === 0 ? "工作区没有未提交的改动" : `${changes.length} 个文件 · 对比工作区基线`)}
          </Text>
          {changes.length === 0 && props.pendingChanges ? <Loading /> : null}
          {changes.length === 0 && !props.pendingChanges && !clientToken ? <Text style={list.empty}>请先完成配对</Text> : null}
          <View style={list.bleed}>
            {changes.map((entry) => {
              const dir = dirName(entry.path);
              const added = entry.added_lines || 0;
              const removed = entry.removed_lines || 0;
              return (
                <Pressable
                  disabled={pendingDiff}
                  key={`${entry.path}-${entry.index_status || ""}-${entry.worktree_status || ""}`}
                  onPress={() => props.onOpenChange(entry)}
                  style={({ pressed }) => [list.rowCompact, (pressed || props.selectedChange === entry.path) && list.pressed]}
                >
                  <Text style={[list.letter, letterStyle(entry)]}>{changeLabel(entry)}</Text>
                  <Text numberOfLines={1} style={[list.body, list.title]}>
                    {baseName(entry.path)}
                    {dir ? <Text style={list.meta}>  {dir}</Text> : null}
                  </Text>
                  {added > 0 || removed > 0 ? (
                    <View style={styles.stats}>
                      {added > 0 ? <Text style={styles.statAdd}>+{added}</Text> : null}
                      {removed > 0 ? <Text style={styles.statDel}>−{removed}</Text> : null}
                    </View>
                  ) : null}
                  {pendingDiff && props.selectedChange === entry.path ? (
                    <ActivityIndicator color={palette.ink3} size="small" />
                  ) : (
                    <Feather color={palette.ink4} name="chevron-right" size={16} />
                  )}
                </Pressable>
              );
            })}
          </View>
        </>
      ) : (
        <>
          <Text style={list.hint}>{props.historyMessage || "Agent 修改文件前自动保存的快照，可以对比或回滚"}</Text>
          {historyCheckpoints.length === 0 ? (
            props.pendingHistory ? <Loading /> : <Text style={list.empty}>{clientToken ? "还没有历史快照" : "请先完成配对"}</Text>
          ) : null}
          <View style={list.bleed}>
            {historyCheckpoints.map((checkpoint, index) => (
              <Fragment key={checkpoint.id}>
                {index > 0 ? <View style={list.dividerFull} /> : null}
                <View style={[list.row, { alignItems: "flex-start", paddingVertical: 12 }]}>
                  <Text style={[list.meta, { paddingTop: 2, width: 58 }]}>{formatShortTime(checkpoint.created_at) || "—"}</Text>
                  <View style={list.body}>
                    <Text numberOfLines={2} style={list.title}>{checkpoint.title || `快照 ${shortID(checkpoint.id)}`}</Text>
                    <Text style={list.sub}>{checkpoint.change_count} 个文件</Text>
                    <View style={{ flexDirection: "row", gap: 20, marginTop: 4 }}>
                      <Pressable
                        disabled={pendingDiff}
                        hitSlop={6}
                        onPress={() => props.onPreviewHistory(checkpoint.id)}
                        style={[list.textButton, pendingDiff && list.disabled]}
                      >
                        <Text style={list.textLink}>对比</Text>
                      </Pressable>
                      <Pressable
                        disabled={props.pendingRevert}
                        hitSlop={6}
                        onPress={() => setConfirmCheckpoint(checkpoint)}
                        style={[list.textButton, props.pendingRevert && list.disabled]}
                      >
                        <Text style={list.textDanger}>回滚到此</Text>
                      </Pressable>
                    </View>
                  </View>
                </View>
              </Fragment>
            ))}
          </View>
        </>
      )}

      <ConfirmModal
        confirmLabel="回滚快照"
        danger
        icon="rotate-ccw"
        message={`确定恢复「${confirmCheckpoint?.title || `快照 ${shortID(confirmCheckpoint?.id || "")}`}」吗？\n将还原 ${confirmCheckpoint?.change_count || 0} 个文件至 ${formatDateTime(confirmCheckpoint?.created_at)} 的状态。`}
        onCancel={() => setConfirmCheckpoint(null)}
        onConfirm={() => {
          if (confirmCheckpoint) {
            props.onRevertHistory(confirmCheckpoint.id);
          }
          setConfirmCheckpoint(null);
        }}
        title="回滚历史快照"
        visible={Boolean(confirmCheckpoint)}
      />
    </View>
  );
}

function Loading() {
  return <SkeletonRows count={4} />;
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    stats: { alignItems: "center", flexDirection: "row", gap: 5 },
    statAdd: { color: palette.diffAdd, fontFamily: fontFamily.mono, fontSize: 11.5, fontWeight: weight.semibold },
    statDel: { color: palette.diffDel, fontFamily: fontFamily.mono, fontSize: 11.5, fontWeight: weight.semibold },
  });
