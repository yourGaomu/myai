import { useEffect, useMemo, useState } from "react";
import { Image, Platform, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SubagentTask } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import type { AgentTurnItem, AgentTurnTimelineStep } from "../../utils/chatRenderItems";
import { usageBreakdown } from "../../utils/tokenUsage";
import { font, palette, radius, weight } from "../../constants/theme";
import { Spinner } from "../common/Spinner";
import { ChatAttachmentCard } from "./ChatAttachmentCard";
import { MarkdownText } from "./MarkdownText";
import { ShimmerLabel } from "./ShimmerLabel";
import { extractFileChanges, TurnChanges } from "./turn/TurnChanges";
import { TurnTimeline } from "./turn/TurnTimeline";

type Props = {
  buttonFeedback: ButtonFeedback;
  onInspectSubagent?: (taskID: string) => void;
  onOpenChanges?: () => void;
  onRegenerate?: () => void;
  subagentTasks?: SubagentTask[];
  turn: AgentTurnItem;
};

// 原生端没有剪贴板依赖（expo-clipboard 未安装），复制按钮只在 web 上显示，避免假装复制成功。
const canCopy = Platform.OS === "web" && typeof navigator !== "undefined" && Boolean(navigator.clipboard?.writeText);

// 一轮 Agent 回答：过程折叠成一行（点开是时间线），正文按文档排版，最后是变更摘要和操作。
export function AgentTurnCard({ buttonFeedback, onInspectSubagent, onOpenChanges, onRegenerate, subagentTasks, turn }: Props) {
  const running = turn.status === "running";
  const steps = useMemo<AgentTurnTimelineStep[]>(() => {
    if (turn.timeline?.length) return turn.timeline;
    const fallback: AgentTurnTimelineStep[] = [];
    if (turn.reasoning?.trim()) fallback.push({ type: "thought", id: `${turn.id}-thought`, text: turn.reasoning.trim() });
    if (turn.tools?.length) fallback.push({ type: "tool_group", id: `${turn.id}-tools`, tools: turn.tools });
    return fallback;
  }, [turn.id, turn.reasoning, turn.timeline, turn.tools]);
  // 运行中默认展开，结束后自动收起；用户手动点过之后以用户的选择为准
  const [manualOpen, setManualOpen] = useState<boolean | null>(null);
  const [copied, setCopied] = useState(false);
  const clock = useRunningClock(running, turn.startedAt);
  const tokens = useMemo(() => usageBreakdown(turn.usage), [turn.usage]);
  const files = useMemo(() => extractFileChanges(turn.tools), [turn.tools]);
  const open = manualOpen ?? running;
  const opCount = turn.tools?.length || 0;
  const doneLabel = [
    turn.status === "paused" ? "已停止" : turn.status === "error" ? "执行出错" : turn.elapsed ? `已处理 ${turn.elapsed}` : "已处理",
    opCount ? `${opCount} 个操作` : "",
  ].filter(Boolean).join(" · ");
  const canRegenerate = Boolean(turn.canRegenerate && onRegenerate);
  // error 消息会被并进当前轮次，但不在 turn.text 里，单独显示，避免错误被吞掉
  // 断线重连每次失败都会追加同样的错误，合并成一条并显示次数
  const errors = useMemo(() => groupErrors(turn.messages), [turn.messages]);
  const hasContent = Boolean(turn.text.trim() || turn.reasoning?.trim());
  const showModel = Boolean(turn.model && turn.model !== "MyAI Agent");

  // 只有错误、没有任何回答内容的轮次（例如连接失败）：显示成一行提示，不画头像和操作栏
  if (!running && !hasContent && steps.length === 0 && errors.length > 0) {
    return (
      <View style={styles.turn}>
        {errors.map((error, index) => <ErrorNotice count={error.count} key={turn.id + "-err-" + index} text={error.text} />)}
      </View>
    );
  }

  const handleCopy = () => {
    const content = (turn.text || turn.reasoning || "").trim();
    if (!content || !canCopy) return;
    void navigator.clipboard.writeText(content).catch(() => undefined);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <View style={styles.turn}>
      <View style={styles.meta}>
        <View style={styles.avatar}>
          <Image source={require("../../../assets/avatar.png")} style={styles.avatarImage} />
          {running ? (
            <View style={styles.avatarSpin}>
              <Spinner size={30} />
            </View>
          ) : null}
        </View>
        <Text style={styles.name}>MyAI</Text>
        {showModel ? <Text numberOfLines={1} style={styles.model}>{turn.model}</Text> : null}
      </View>

      {running || steps.length > 0 ? (
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ expanded: open }}
          disabled={steps.length === 0}
          onPress={() => setManualOpen(!open)}
          style={({ pressed }) => [styles.procHead, pressed && styles.pressed]}
        >
          {running ? (
            <>
              <ShimmerLabel containerStyle={styles.shimmer} label="正在处理" />
              {clock ? <Text style={styles.procLabel}>· {clock}</Text> : null}
            </>
          ) : (
            <Text style={[styles.procLabel, turn.status === "error" && styles.procLabelError]}>{doneLabel}</Text>
          )}
          {!running && steps.length > 0 ? (
            <Feather color={palette.ink3} name={open ? "chevron-down" : "chevron-right"} size={15} />
          ) : null}
        </Pressable>
      ) : null}

      {open && steps.length > 0 ? (
        <TurnTimeline buttonFeedback={buttonFeedback} onInspectSubagent={onInspectSubagent} steps={steps} subagentTasks={subagentTasks} />
      ) : null}

      {turn.text ? <MarkdownText text={turn.text} /> : null}
      {turn.attachments?.length ? (
        <View style={styles.attachments}>
          {turn.attachments.map((attachment, index) => (
            <ChatAttachmentCard attachment={attachment} buttonFeedback={buttonFeedback} key={`${turn.id}-att-${index}`} />
          ))}
        </View>
      ) : null}

      {errors.map((error, index) => <ErrorNotice count={error.count} key={turn.id + "-err-" + index} text={error.text} />)}

      {!running && (turn.status === "paused" || turn.status === "error") ? (
        <View style={styles.note}>
          <Text style={styles.noteText}>{turn.status === "paused" ? "已停止。" : "执行出错。"}</Text>
          {canRegenerate ? (
            <Pressable accessibilityRole="button" hitSlop={10} onPress={onRegenerate}>
              <Text style={styles.noteLink}>重新生成</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}

      {!running ? <TurnChanges files={files} onOpenChanges={onOpenChanges} /> : null}

      {!running && ((canCopy && hasContent) || canRegenerate || tokens) ? (
        <View style={styles.actions}>
          {canCopy && hasContent ? (
            <Pressable accessibilityLabel="复制回答" onPress={handleCopy} style={({ pressed }) => [styles.action, pressed && styles.pressed]}>
              <Feather color={palette.ink3} name={copied ? "check" : "copy"} size={17} />
            </Pressable>
          ) : null}
          {canRegenerate ? (
            <Pressable accessibilityLabel="重新生成" onPress={onRegenerate} style={({ pressed }) => [styles.action, pressed && styles.pressed]}>
              <Feather color={palette.ink3} name="rotate-ccw" size={17} />
            </Pressable>
          ) : null}
          {tokens ? <Text style={styles.tokens}>{tokens.total} tokens</Text> : null}
        </View>
      ) : null}
    </View>
  );
}

function groupErrors(messages: AgentTurnItem["messages"]) {
  const groups: { count: number; text: string }[] = [];
  for (const message of messages) {
    const text = message.role === "error" ? message.text.trim() : "";
    if (!text) continue;
    const same = groups.find((group) => group.text === text);
    if (same) same.count += 1;
    else groups.push({ count: 1, text });
  }
  return groups;
}

function ErrorNotice({ count, text }: { count: number; text: string }) {
  return (
    <View style={styles.error}>
      <Feather color={palette.err} name="alert-circle" size={15} style={styles.errorIcon} />
      <Text selectable style={styles.errorText}>{text}</Text>
      {count > 1 ? <Text style={styles.errorCount}>×{count}</Text> : null}
    </View>
  );
}

function useRunningClock(running: boolean, startedAt?: string) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!running) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [running]);
  const start = startedAt ? Date.parse(startedAt) : Number.NaN;
  if (!running || !Number.isFinite(start)) return "";
  const seconds = Math.max(0, Math.floor((now - start) / 1000));
  return seconds >= 7200 ? "" : `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

const styles = StyleSheet.create({
  turn: { alignSelf: "stretch", gap: 10, width: "100%" },
  meta: { alignItems: "center", flexDirection: "row", gap: 8 },
  avatar: { height: 24, width: 24 },
  avatarImage: { backgroundColor: palette.bubble, borderRadius: radius.pill, height: 24, width: 24 },
  avatarSpin: { left: -3, position: "absolute", top: -3 },
  name: { color: palette.ink1, fontSize: font.caption, fontWeight: weight.semibold },
  model: { color: palette.ink3, flexShrink: 1, fontSize: font.caption },
  procHead: {
    alignItems: "center",
    alignSelf: "flex-start",
    borderRadius: radius.xs + 2,
    flexDirection: "row",
    gap: 6,
    marginLeft: -6,
    minHeight: 32,
    paddingHorizontal: 6,
  },
  pressed: { backgroundColor: palette.pressed },
  shimmer: { alignSelf: "center", minWidth: 0 },
  procLabel: { color: palette.ink2, fontSize: font.label + 0.5 },
  procLabelError: { color: palette.err },
  attachments: { gap: 8 },
  note: { alignItems: "center", flexDirection: "row", gap: 6 },
  noteText: { color: palette.ink2, fontSize: font.label },
  noteLink: { color: palette.brandLight, fontSize: font.label, fontWeight: weight.medium },
  actions: { alignItems: "center", flexDirection: "row", gap: 2, marginLeft: -9 },
  action: { alignItems: "center", borderRadius: radius.pill, height: 38, justifyContent: "center", width: 38 },
  tokens: { color: palette.ink3, fontSize: font.caption, marginLeft: 6 },
  error: { alignItems: "flex-start", backgroundColor: palette.errWash, borderRadius: radius.md, flexDirection: "row", gap: 8, paddingHorizontal: 12, paddingVertical: 10 },
  errorIcon: { marginTop: 2 },
  errorText: { color: palette.err, flex: 1, fontSize: font.label + 0.5, lineHeight: 21 },
  errorCount: { color: palette.err, fontSize: font.caption, lineHeight: 21, opacity: 0.75 },
});
