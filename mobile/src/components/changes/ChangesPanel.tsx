import { Fragment, useState } from "react";
import { ActivityIndicator, Pressable, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ChangeEntry, HistoryCheckpoint } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { changeLabel } from "../../utils/changes";
import { formatShortTime } from "../../utils/format";
import { shortID } from "../../utils/ids";
import { baseName, dirName } from "../../utils/toolDisplay";
import { palette } from "../../constants/theme";
import { list } from "../common/listStyles";
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

function letterStyle(entry: ChangeEntry) {
  if (entry.deleted) return list.letterDel;
  if (entry.untracked || entry.status === "added") return list.letterAdd;
  if (entry.renamed) return list.letterRen;
  return list.letterMod;
}

// 变更页：未提交（对比工作区基线）/ 历史快照两组。顶栏刷新会同时刷新两组。
export function ChangesPanel(props: Props) {
  const { changes, clientToken, historyCheckpoints, pendingDiff } = props;
  const [tab, setTab] = useState<"now" | "history">("now");

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
                      <Pressable disabled={pendingDiff} hitSlop={6} onPress={() => props.onPreviewHistory(checkpoint.id)} style={[list.textButton, pendingDiff && list.disabled]}>
                        <Text style={list.textLink}>对比</Text>
                      </Pressable>
                      <Pressable disabled={props.pendingRevert} hitSlop={6} onPress={() => props.onRevertHistory(checkpoint.id)} style={[list.textButton, props.pendingRevert && list.disabled]}>
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
    </View>
  );
}

function Loading() {
  return (
    <View style={list.loading}>
      <ActivityIndicator color={palette.ink2} size="small" />
      <Text style={list.loadingText}>正在加载…</Text>
    </View>
  );
}
