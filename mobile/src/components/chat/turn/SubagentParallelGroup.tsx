import { useEffect, useMemo, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SubagentTask } from "../../../protocol";
import type { ToolCallStep } from "../../../utils/chatRenderItems";
import { toolTarget } from "../../../utils/toolDisplay";
import { font, radius, weight } from "../../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../../theme/ThemeProvider";
import { CharacterAvatar } from "../../common/CharacterAvatar";
import { subagentStatusLabel } from "./SubagentSummary";

type SubagentEntry = {
  target: string;
  task?: SubagentTask;
  tool: ToolCallStep;
};

type Props = {
  entries: SubagentEntry[];
  onInspectSubagent?: (taskID: string) => void;
};

function formatSeconds(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  const s = ms / 1000;
  return s < 10 ? `${s.toFixed(1)}s` : `${Math.round(s)}s`;
}

function entryElapsedMs(entry: SubagentEntry): number {
  const startRaw = entry.tool.createdAt || entry.task?.created_at;
  const endRaw = entry.tool.completedAt || entry.task?.updated_at;
  const start = startRaw ? Date.parse(startRaw) : Number.NaN;
  const end = endRaw ? Date.parse(endRaw) : Number.NaN;
  if (Number.isFinite(start) && Number.isFinite(end) && end >= start) {
    return end - start;
  }
  return 0;
}

// 将并发触发的多条子代理调用聚合成一张紧凑的可折叠并行总览卡（点击任一行直达实时详情）。
export function SubagentParallelGroup({ entries, onInspectSubagent }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);

  const summary = useMemo(() => {
    let doneCount = 0;
    let runningCount = 0;
    let failedCount = 0;
    let minStart = Number.POSITIVE_INFINITY;
    let maxEnd = 0;

    for (const item of entries) {
      const status = item.task?.status || (item.tool.status === "running" ? "running" : item.tool.status === "error" ? "failed" : "succeeded");
      if (status === "running" || status === "queued" || status === "waiting_subagents" || status === "waiting_permission") {
        runningCount += 1;
      } else if (status === "failed" || status === "canceled") {
        failedCount += 1;
      } else {
        doneCount += 1;
      }
      const start = Date.parse(item.tool.createdAt || item.task?.created_at || "");
      const end = Date.parse(item.tool.completedAt || item.task?.updated_at || "");
      if (Number.isFinite(start)) minStart = Math.min(minStart, start);
      if (Number.isFinite(end)) maxEnd = Math.max(maxEnd, end);
    }

    const totalMs = Number.isFinite(minStart) && maxEnd > minStart ? maxEnd - minStart : 0;
    return { doneCount, failedCount, runningCount, totalMs };
  }, [entries]);

  const anyRunning = summary.runningCount > 0;
  const [expanded, setExpanded] = useState(anyRunning || summary.failedCount > 0);

  useEffect(() => {
    if (anyRunning) {
      setExpanded(true);
    }
  }, [anyRunning]);

  const headline = anyRunning
    ? `子代理并行检索 · ${summary.doneCount}/${entries.length} 完成`
    : `子代理并行检索 · ${entries.length} 个任务${summary.failedCount > 0 ? `（${summary.failedCount} 异常）` : "完成"}`;
  const durationLabel = anyRunning ? "执行中" : formatSeconds(summary.totalMs);

  return (
    <View style={[styles.card, anyRunning && styles.cardActive]}>
      <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => [styles.head, pressed && styles.pressed]}>
        <View style={styles.avatarBox}>
          <CharacterAvatar circular size={24} slot={anyRunning ? "subagent" : "done"} variant="avatar" />
        </View>
        <View style={styles.headBody}>
          <Text numberOfLines={1} style={styles.headline}>
            {headline}
          </Text>
          <Text numberOfLines={1} style={styles.subline}>
            {entries.map((item) => item.task?.agent_nickname || item.target || "子代理").slice(0, 3).join(" / ")}
          </Text>
        </View>
        {durationLabel ? <Text style={styles.duration}>{durationLabel}</Text> : null}
        <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={14} />
      </Pressable>

      {expanded ? (
        <View style={styles.rows}>
          {entries.map((entry, idx) => {
            const st = entry.task?.status || (entry.tool.status === "running" ? "running" : entry.tool.status === "error" ? "failed" : "succeeded");
            const dotColor =
              st === "failed" || st === "canceled"
                ? palette.err
                : st === "running" || st === "queued"
                  ? palette.brandLight
                  : st === "waiting_permission" || st === "waiting_subagents"
                    ? palette.warn
                    : palette.ok;
            const title = entry.task?.agent_nickname || entry.task?.title || entry.target || `子任务 #${idx + 1}`;
            const elapsed = formatSeconds(entryElapsedMs(entry)) || subagentStatusLabel(st);
            const taskID = entry.task?.id;

            return (
              <Pressable
                accessibilityRole="button"
                disabled={!taskID || !onInspectSubagent}
                key={entry.tool.id || `${idx}`}
                onPress={() => taskID && onInspectSubagent?.(taskID)}
                style={({ pressed }) => [styles.row, pressed && taskID && styles.pressed]}
              >
                <View style={[styles.dot, { backgroundColor: dotColor }]} />
                <Text numberOfLines={1} style={styles.rowTitle}>
                  {title}
                </Text>
                <Text style={styles.rowMeta}>{elapsed}</Text>
                {taskID && onInspectSubagent ? <Feather color={palette.ink4} name="chevron-right" size={13} /> : null}
              </Pressable>
            );
          })}
        </View>
      ) : null}
    </View>
  );
}

export function subagentTargetLabel(tool: ToolCallStep): string {
  return toolTarget(tool.name, tool.arguments);
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    card: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      marginBottom: 6,
      marginLeft: 35,
      marginTop: 2,
      overflow: "hidden",
    },
    cardActive: {
      borderColor: palette.brandDeep,
    },
    head: {
      alignItems: "center",
      flexDirection: "row",
      gap: 9,
      paddingHorizontal: 11,
      paddingVertical: 9,
    },
    avatarBox: {
      alignItems: "center",
      height: 26,
      justifyContent: "center",
      width: 26,
    },
    headBody: {
      flex: 1,
      gap: 1,
      minWidth: 0,
    },
    headline: {
      color: palette.ink1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    subline: {
      color: palette.ink3,
      fontSize: 11.5,
    },
    duration: {
      color: palette.ink3,
      fontSize: font.caption,
      fontWeight: weight.medium,
    },
    rows: {
      borderTopColor: palette.hairline,
      borderTopWidth: 1,
      paddingHorizontal: 11,
      paddingVertical: 6,
    },
    row: {
      alignItems: "center",
      borderRadius: radius.sm,
      flexDirection: "row",
      gap: 8,
      minHeight: 34,
      paddingHorizontal: 4,
    },
    dot: {
      borderRadius: radius.pill,
      height: 7,
      width: 7,
    },
    rowTitle: {
      color: palette.ink2,
      flex: 1,
      fontSize: font.label - 0.5,
    },
    rowMeta: {
      color: palette.ink3,
      fontSize: font.caption - 0.5,
    },
    pressed: {
      backgroundColor: palette.pressed,
    },
  });
