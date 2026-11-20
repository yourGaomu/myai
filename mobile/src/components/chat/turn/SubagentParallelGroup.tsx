import { useEffect, useMemo, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SubagentTask } from "../../../protocol";
import type { ToolCallStep } from "../../../utils/chatRenderItems";
import { subagentLaunchLabel, subagentLaunchState } from "../../../utils/subagentLaunch";
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
  turnRunning: boolean;
};

function entryStatus(entry: SubagentEntry, turnRunning: boolean) {
  const launch = subagentLaunchState(entry.tool, entry.task, turnRunning);
  const status = entry.task?.status;
  if (launch === "starting") return { label: subagentLaunchLabel.starting, tone: "active" } as const;
  if (launch === "failed" || launch === "canceled") return { label: subagentLaunchLabel[launch], tone: "error" } as const;
  if (status && launch !== "skipped") {
    const tone = status === "succeeded" ? "done"
      : status === "failed" || status === "canceled" ? "error"
      : status.startsWith("waiting_") ? "waiting" : "active";
    return { label: subagentStatusLabel(status), tone } as const;
  }
  return { label: subagentLaunchLabel[launch], tone: "neutral" } as const;
}

// 将并发触发的多条子代理调用聚合成一张紧凑的可折叠并行总览卡（点击任一行直达实时详情）。
export function SubagentParallelGroup({ entries, onInspectSubagent, turnRunning }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const statuses = useMemo(() => entries.map((entry) => entryStatus(entry, turnRunning)), [entries, turnRunning]);
  const doneCount = statuses.filter((status) => status.tone === "done").length;
  const anyRunning = statuses.some((status) => status.tone === "active" || status.tone === "waiting");
  const failures = statuses.filter((status) => status.tone === "error").length;
  const [expanded, setExpanded] = useState(anyRunning || doneCount < entries.length);

  useEffect(() => {
    if (anyRunning) {
      setExpanded(true);
    }
  }, [anyRunning]);

  const headline = anyRunning || doneCount < entries.length
    ? `子代理并行检索 · ${doneCount}/${entries.length} 完成${failures ? ` · ${failures} 项异常` : ""}`
    : `子代理并行检索 · ${entries.length} 个任务完成`;

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
        {anyRunning ? <Text style={styles.duration}>执行中</Text> : null}
        <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={14} />
      </Pressable>

      {expanded ? (
        <View style={styles.rows}>
          {entries.map((entry, idx) => {
            const status = statuses[idx];
            const dotColor = status.tone === "error" ? palette.err
              : status.tone === "done" ? palette.ok
              : status.tone === "waiting" ? palette.warn
              : status.tone === "active" ? palette.brandLight : palette.ink4;
            const title = entry.task?.agent_nickname || entry.task?.title || entry.target || `子任务 #${idx + 1}`;
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
                <Text style={styles.rowMeta}>{status.label}</Text>
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
