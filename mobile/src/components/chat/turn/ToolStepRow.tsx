import { useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SubagentTask } from "../../../protocol";
import type { ButtonFeedback } from "../../../types/ui";
import type { ToolCallStep } from "../../../utils/chatRenderItems";
import { parseSharedAsset } from "../../../utils/toolAssets";
import { parseToolArgs, toolCommand, toolIcon, toolKind, toolTarget, toolVerb } from "../../../utils/toolDisplay";
import { fontFamily, font, palette, radius, weight } from "../../../constants/theme";
import { Spinner } from "../../common/Spinner";
import { SharedAssetCard } from "../SharedAssetCard";
import { SubagentSummary } from "./SubagentSummary";

type Props = {
  buttonFeedback: ButtonFeedback;
  onInspectSubagent?: (taskID: string) => void;
  subagentTasks?: SubagentTask[];
  tool: ToolCallStep;
};

// 时间线里的一次工具调用：图标 + 动词 + 对象，点开看请求参数和返回内容。
export function ToolStepRow({ buttonFeedback, onInspectSubagent, subagentTasks, tool }: Props) {
  const failed = Boolean(tool.error || tool.status === "error");
  const running = tool.status === "running";
  const [open, setOpen] = useState(failed);
  const kind = toolKind(tool.name);
  const target = useMemo(() => toolTarget(tool.name, tool.arguments), [tool.arguments, tool.name]);
  const request = useMemo(() => requestText(tool), [tool]);
  const sharedAsset = parseSharedAsset(tool.name, tool.result || "");
  const output = tool.error || tool.result || "";

  // 按 parent_tool_call_id 精确配对子代理任务（并发任务不按顺序或工具名配对）
  const task = useMemo(() => {
    const callID = tool.toolCallID || tool.id;
    return subagentTasks?.find((t) => t.parent_tool_call_id && (t.parent_tool_call_id === callID || t.parent_tool_call_id === tool.id));
  }, [subagentTasks, tool.id, tool.toolCallID]);

  const verbStyle = failed ? styles.verbError : running ? styles.verbRunning : styles.verbDone;

  return (
    <View>
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        onPress={() => setOpen((value) => !value)}
        style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
      >
        <View style={[styles.icon, failed && styles.iconError]}>
          {running ? (
            <Spinner size={14} />
          ) : (
            <Feather color={failed ? palette.err : palette.ink3} name={toolIcon(kind)} size={15} />
          )}
        </View>
        <Text numberOfLines={1} style={styles.label}>
          <Text style={verbStyle}>{toolVerb(kind, failed ? "error" : running ? "running" : "done")}</Text>
          <Text style={kind === "other" ? null : styles.mono}> {target}</Text>
        </Text>
        {!running && tool.duration ? <Text style={styles.meta}>{tool.duration}</Text> : null}
        <Feather color={palette.ink4} name={open ? "chevron-down" : "chevron-right"} size={14} />
      </Pressable>

      {task ? (
        <SubagentSummary onInspect={onInspectSubagent} task={task} />
      ) : kind === "subagent_spawn" && running ? (
        <View style={styles.spawning}>
          <Spinner color={palette.info} size={12} />
          <Text style={styles.spawningText}>子代理启动中…</Text>
        </View>
      ) : null}

      {open ? (
        <View style={styles.detail}>
          <Text style={styles.detailLabel}>请求 · {tool.name}</Text>
          <Text selectable style={styles.code}>
            {request || "（无请求参数）"}
          </Text>
          <View style={styles.divider} />
          <Text style={[styles.detailLabel, failed && styles.detailLabelError]}>
            {failed ? `执行异常${tool.errorCode ? ` · ${tool.errorCode}` : ""}` : running ? "执行中" : "返回"}
            {tool.truncated ? " · 已截断" : ""}
          </Text>
          {sharedAsset && !failed ? (
            <SharedAssetCard asset={sharedAsset} buttonFeedback={buttonFeedback} />
          ) : (
            <ScrollView nestedScrollEnabled showsVerticalScrollIndicator style={styles.outputScroll}>
              <Text selectable style={[styles.code, failed && styles.codeError]}>
                {output || (running ? "正在等待工具返回…" : "（无返回内容）")}
              </Text>
            </ScrollView>
          )}
        </View>
      ) : null}
    </View>
  );
}

function requestText(tool: ToolCallStep) {
  const command = toolKind(tool.name) === "shell" ? toolCommand(tool.arguments) : "";
  if (command) {
    return `$ ${command}`;
  }
  const args = parseToolArgs(tool.arguments);
  if (typeof args === "string") return args;
  return args ? JSON.stringify(args, null, 2) : String(tool.arguments || "").trim();
}

const styles = StyleSheet.create({
  row: { alignItems: "center", borderRadius: radius.xs + 2, flexDirection: "row", gap: 8, minHeight: 36, paddingVertical: 4 },
  rowPressed: { backgroundColor: palette.pressed },
  icon: {
    alignItems: "center",
    backgroundColor: palette.canvas,
    borderRadius: radius.xs + 2,
    height: 27,
    justifyContent: "center",
    width: 27,
  },
  iconError: { backgroundColor: palette.errWash },
  label: { color: palette.ink2, flex: 1, fontSize: font.reading - 2, minWidth: 0 },
  verbDone: { color: palette.ink1 },
  verbRunning: { color: palette.brandLight },
  verbError: { color: palette.err },
  mono: { fontFamily: fontFamily.mono, fontSize: font.label - 0.5 },
  meta: { color: palette.ink3, fontSize: font.caption },
  spawning: { alignItems: "center", flexDirection: "row", gap: 8, marginLeft: 35, paddingVertical: 6 },
  spawningText: { color: palette.info, fontSize: font.label },
  detail: {
    backgroundColor: palette.well,
    borderRadius: radius.md - 2,
    gap: 6,
    marginBottom: 8,
    marginLeft: 35,
    marginTop: 2,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  detailLabel: { color: palette.ink3, fontSize: font.caption, fontWeight: weight.medium },
  detailLabelError: { color: palette.err },
  divider: { backgroundColor: palette.hairline, height: 1, marginVertical: 2 },
  code: { color: palette.ink1, fontFamily: fontFamily.mono, fontSize: font.caption, lineHeight: 19 },
  codeError: { color: palette.err },
  outputScroll: { maxHeight: 240 },
});
