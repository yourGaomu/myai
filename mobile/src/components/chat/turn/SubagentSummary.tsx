import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SubagentTask } from "../../../protocol";
import { font, palette, radius, weight } from "../../../constants/theme";

type Props = {
  onInspect?: (taskID: string) => void;
  task: SubagentTask;
};

// 工具行下方的子代理摘要：状态 + 最近输出，点开进入只读实时详情。
export function SubagentSummary({ onInspect, task }: Props) {
  const snippet = task.result || task.reasoning || task.instruction;
  return (
    <Pressable
      accessibilityRole="button"
      disabled={!onInspect}
      onPress={() => onInspect?.(task.id)}
      style={({ pressed }) => [styles.card, pressed && styles.pressed]}
    >
      <View style={styles.head}>
        <View style={[styles.dot, dotStyle(task.status)]} />
        <Text numberOfLines={1} style={styles.name}>
          {task.agent_nickname || task.title || "子代理"}
        </Text>
        <Text style={styles.status}>{subagentStatusLabel(task.status)}</Text>
        {onInspect ? (
          <>
            <Text style={styles.link}>实时详情</Text>
            <Feather color={palette.ink4} name="chevron-right" size={14} />
          </>
        ) : null}
      </View>
      {snippet ? (
        <Text numberOfLines={2} style={styles.snippet}>
          {snippet}
        </Text>
      ) : null}
    </Pressable>
  );
}

function dotStyle(status?: string) {
  if (status === "running") return styles.dotRunning;
  if (status === "succeeded") return styles.dotOk;
  if (status === "failed") return styles.dotFailed;
  if (status === "waiting_subagents" || status === "waiting_permission") return styles.dotWaiting;
  return null;
}

export function subagentStatusLabel(status?: string) {
  switch (status) {
    case "queued":
      return "排队中";
    case "running":
      return "执行中";
    case "waiting_subagents":
      return "等待子任务";
    case "waiting_permission":
      return "等待授权";
    case "succeeded":
      return "已完成";
    case "failed":
      return "失败";
    case "canceled":
      return "已取消";
    default:
      return status || "运行中";
  }
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: palette.surface1,
    borderRadius: radius.md - 2,
    gap: 4,
    marginBottom: 6,
    marginLeft: 35,
    marginTop: 2,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  pressed: {
    backgroundColor: palette.pressed,
  },
  head: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  dot: {
    backgroundColor: palette.ink4,
    borderRadius: radius.pill,
    height: 7,
    width: 7,
  },
  dotRunning: {
    backgroundColor: palette.brand,
  },
  dotOk: {
    backgroundColor: palette.ok,
  },
  dotFailed: {
    backgroundColor: palette.err,
  },
  dotWaiting: {
    backgroundColor: palette.warn,
  },
  name: {
    color: palette.ink1,
    flex: 1,
    fontSize: font.label + 0.5,
    fontWeight: weight.medium,
  },
  status: {
    color: palette.ink3,
    fontSize: font.caption,
  },
  link: {
    color: palette.brandLight,
    fontSize: font.label - 0.5,
    marginLeft: 4,
  },
  snippet: {
    color: palette.ink2,
    fontSize: font.caption + 0.5,
    lineHeight: 19,
  },
});
