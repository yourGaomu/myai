import { useEffect, useMemo, useState } from "react";
import { Platform, Pressable, StyleSheet, Text, View } from "react-native";

import type { AgentRunEvent, AgentRunSnapshot } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { ThinkingReasoning } from "./ThinkingReasoning";

type Props = {
  buttonFeedback: ButtonFeedback;
  snapshot: AgentRunSnapshot;
};

export function AgentRunTimeline({ buttonFeedback, snapshot }: Props) {
  const [expanded, setExpanded] = useState(snapshot.run.status === "running");
  const elapsed = useRunElapsed(snapshot);
  const reasoning = useMemo(
    () => snapshot.events
      .filter((event) => event.type === "reasoning")
      .map(eventContent)
      .filter(Boolean)
      .join("\n\n"),
    [snapshot.events],
  );
  const events = useMemo(() => {
    const visible = snapshot.events.filter((event) => event.type !== "reasoning");
    if (snapshot.run.status !== "running" && !visible.some(isTerminalEvent)) {
      visible.push(terminalEventFromRun(snapshot));
    }
    return visible;
  }, [snapshot]);

  useEffect(() => {
    if (snapshot.run.status === "running") {
      setExpanded(true);
    }
  }, [snapshot.run.status]);

  return (
    <View style={styles.container}>
      <Pressable
        accessibilityLabel={expanded ? "收起执行记录" : "展开执行记录"}
        onPress={() => setExpanded((value) => !value)}
        style={({ pressed }) => buttonFeedback(styles.header, pressed)}
      >
        <View style={[styles.statusDot, statusDotStyle(snapshot.run.status)]} />
        <View style={styles.headerText}>
          <Text numberOfLines={1} style={styles.title}>{snapshot.run.title || runKindLabel(snapshot.run.kind)}</Text>
          <Text style={styles.subtitle}>{runMeta(snapshot)} / {elapsed}</Text>
        </View>
        <Text style={styles.collapseIcon}>{expanded ? "-" : "+"}</Text>
      </Pressable>

      {expanded ? (
        <View style={styles.body}>
          {reasoning ? (
            <ThinkingReasoning
              buttonFeedback={buttonFeedback}
              finishedAt={snapshot.run.finished_at}
              reasoning={reasoning}
              running={snapshot.run.status === "running"}
              startedAt={snapshot.run.started_at}
            />
          ) : null}

          {events.length === 0 ? (
            reasoning ? null : <Text style={styles.emptyText}>等待执行活动...</Text>
          ) : (
            events.map((event, index) => (
              <TimelineEvent
                event={event}
                key={`${event.id}-${event.sequence}`}
                last={index === events.length - 1}
              />
            ))
          )}
          {snapshot.run.error_message ? <Text style={styles.runError}>{snapshot.run.error_message}</Text> : null}
        </View>
      ) : null}
    </View>
  );
}

function TimelineEvent({ event, last }: { event: AgentRunEvent; last: boolean }) {
  const expandable = event.type === "tool_call" || event.type === "tool_result" || event.type === "permission";
  const [expanded, setExpanded] = useState(event.type === "tool_result" && isFailure(event.status));
  const content = eventContent(event);

  return (
    <View style={styles.eventRow}>
      <View style={styles.rail}>
        <View style={[styles.eventDot, eventDotStyle(event)]} />
        {!last ? <View style={styles.railLine} /> : null}
      </View>
      <View style={styles.eventContent}>
        <Pressable disabled={!expandable} onPress={() => setExpanded((value) => !value)} style={styles.eventHeader}>
          <Text style={styles.eventBadge}>{eventLabel(event)}</Text>
          <Text numberOfLines={1} style={styles.eventTitle}>{eventTitle(event)}</Text>
          {event.current_step && event.total_steps ? (
            <Text style={styles.stepText}>{event.current_step}/{event.total_steps}</Text>
          ) : null}
          {expandable ? <Text style={styles.eventToggle}>{expanded ? "-" : "+"}</Text> : null}
        </Pressable>

        {event.type === "plan_update" && content ? <Text style={styles.eventText}>{content}</Text> : null}
        {event.type === "progress" && content ? <Text style={styles.eventText}>{content}</Text> : null}
        {isTerminalEvent(event) && content ? <Text style={isFailure(event.status) ? styles.errorText : styles.eventText}>{content}</Text> : null}

        {expandable && expanded ? (
          <View style={styles.details}>
            {event.arguments ? (
              <View style={styles.detailSection}>
                <Text style={styles.detailLabel}>参数</Text>
                <Text style={styles.codeText}>{event.arguments}</Text>
              </View>
            ) : null}
            {content ? (
              <View style={styles.detailSection}>
                <Text style={styles.detailLabel}>{isFailure(event.status) ? "错误" : "结果"}</Text>
                <Text style={isFailure(event.status) ? styles.errorCodeText : styles.codeText}>{content}</Text>
              </View>
            ) : null}
            {event.truncated ? <Text style={styles.truncatedText}>输出已截断</Text> : null}
          </View>
        ) : null}
      </View>
    </View>
  );
}

function useRunElapsed(snapshot: AgentRunSnapshot) {
  const running = snapshot.run.status === "running";
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!running) {
      return undefined;
    }
    const timer = setInterval(() => setTick((value) => value + 1), 1000);
    return () => clearInterval(timer);
  }, [running]);
  const start = Date.parse(snapshot.run.started_at);
  const finish = snapshot.run.finished_at ? Date.parse(snapshot.run.finished_at) : Date.now();
  if (!Number.isFinite(start) || !Number.isFinite(finish)) {
    return "0秒";
  }
  return formatDuration(Math.max(0, finish - start));
}

function terminalEventFromRun(snapshot: AgentRunSnapshot): AgentRunEvent {
  const type = terminalEventType(snapshot.run.status);
  return {
    id: `${snapshot.run.id}-terminal`,
    run_id: snapshot.run.id,
    session_id: snapshot.run.session_id,
    sequence: snapshot.run.last_sequence || snapshot.events.length + 1,
    type,
    title: type === "completed" ? "执行完成" : eventTypeLabel(type),
    content: snapshot.run.error_message,
    status: snapshot.run.status,
    current_step: snapshot.run.current_step,
    total_steps: snapshot.run.total_steps,
    created_at: snapshot.run.finished_at || snapshot.run.started_at,
  };
}

function terminalEventType(status: AgentRunSnapshot["run"]["status"]): AgentRunEvent["type"] {
  switch (status) {
    case "failed": return "failed";
    case "paused": return "paused";
    case "canceled": return "canceled";
    default: return "completed";
  }
}

function formatDuration(durationMs: number) {
  const seconds = Math.floor(durationMs / 1000);
  if (seconds < 60) {
    return `${seconds}秒`;
  }
  const minutes = Math.floor(seconds / 60);
  return `${minutes}分 ${seconds % 60}秒`;
}

function runMeta(snapshot: AgentRunSnapshot) {
  const { run } = snapshot;
  const status = runStatusLabel(run.status);
  if (run.total_steps) {
    return `${status} / 第 ${run.current_step || 0}/${run.total_steps} 步`;
  }
  return `${status} / ${snapshot.events.length} 个事件`;
}

function runKindLabel(kind: string) {
  switch (kind) {
    case "plan": return "计划执行";
    case "regenerate": return "重新生成回复";
    case "internal": return "智能体任务";
    default: return "智能体执行";
  }
}

function eventLabel(event: AgentRunEvent) {
  switch (event.type) {
    case "reasoning": return "思考";
    case "tool_call": return "调用";
    case "tool_result": return isFailure(event.status) ? "错误" : "结果";
    case "permission": return "权限";
    case "plan_update": return "计划";
    case "failed": return "失败";
    case "paused": return "暂停";
    case "canceled": return "停止";
    case "completed": return "完成";
    default: return "步骤";
  }
}

function eventTitle(event: AgentRunEvent) {
  if (event.tool_name) {
    return event.tool_name;
  }
  return event.title || eventTypeLabel(event.type);
}

function runStatusLabel(status: string) {
  switch (status) {
    case "succeeded": return "已完成";
    case "failed": return "失败";
    case "paused": return "已暂停";
    case "canceled": return "已停止";
    case "running": return "执行中";
    default: return status;
  }
}

function eventTypeLabel(type: string) {
  switch (type) {
    case "tool_call": return "工具调用";
    case "tool_result": return "工具结果";
    case "plan_update": return "计划更新";
    case "permission": return "权限请求";
    case "progress": return "执行进度";
    case "completed": return "执行完成";
    case "failed": return "执行失败";
    case "paused": return "执行暂停";
    case "canceled": return "执行停止";
    default: return type.replace(/_/g, " ");
  }
}

function eventContent(event: AgentRunEvent) {
  return event.error_message || event.content || event.error_code || "";
}

function isTerminalEvent(event: AgentRunEvent) {
  return event.type === "completed" || event.type === "failed" || event.type === "paused" || event.type === "canceled";
}

function isFailure(status?: string) {
  return status === "failed" || status === "error" || status === "denied" || status === "timeout" || status === "canceled";
}

function statusDotStyle(status: string) {
  switch (status) {
    case "succeeded": return styles.dotSuccess;
    case "failed": return styles.dotError;
    case "paused": return styles.dotPaused;
    case "canceled": return styles.dotCanceled;
    default: return styles.dotRunning;
  }
}

function eventDotStyle(event: AgentRunEvent) {
  if (event.type === "failed" || isFailure(event.status)) {
    return styles.dotError;
  }
  if (event.type === "completed" || event.status === "success" || event.status === "succeeded" || event.status === "allowed") {
    return styles.dotSuccess;
  }
  if (event.type === "reasoning") {
    return styles.dotReasoning;
  }
  return styles.dotRunning;
}

const styles = StyleSheet.create({
  container: {
    alignSelf: "stretch",
    backgroundColor: "#f1eee7",
    borderColor: "#d7cfc2",
    borderRadius: 14,
    borderWidth: 1,
    overflow: "hidden",
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    gap: 9,
    minHeight: 50,
    paddingHorizontal: 12,
    paddingVertical: 7,
  },
  statusDot: { borderColor: "#12100e", borderRadius: 7, borderWidth: 2, height: 14, width: 14 },
  dotRunning: { backgroundColor: "#ffd84f" },
  dotSuccess: { backgroundColor: "#8fd17f" },
  dotError: { backgroundColor: "#ff7f68" },
  dotPaused: { backgroundColor: "#4fd7ee" },
  dotCanceled: { backgroundColor: "#b8b2aa" },
  dotReasoning: { backgroundColor: "#c9a7ed" },
  headerText: { flex: 1, minWidth: 0 },
  title: { color: "#12100e", fontSize: 13, fontWeight: "900" },
  subtitle: { color: "#6c665f", fontSize: 10, fontWeight: "700", marginTop: 2 },
  collapseIcon: { color: "#12100e", fontSize: 20, fontWeight: "900", textAlign: "center", width: 24 },
  body: { borderTopColor: "#d7cfc2", borderTopWidth: 1, paddingHorizontal: 12, paddingVertical: 10 },
  emptyText: { color: "#6c665f", fontSize: 11, paddingVertical: 5 },
  eventRow: { flexDirection: "row", minHeight: 38 },
  rail: { alignItems: "center", marginRight: 8, width: 14 },
  eventDot: { borderColor: "#12100e", borderRadius: 6, borderWidth: 2, height: 12, marginTop: 4, width: 12, zIndex: 1 },
  railLine: { backgroundColor: "#c9c0b3", flex: 1, marginBottom: -4, marginTop: -1, width: 1 },
  eventContent: { flex: 1, minWidth: 0, paddingBottom: 11 },
  eventHeader: { alignItems: "center", flexDirection: "row", gap: 6, minHeight: 22 },
  eventBadge: { color: "#6c665f", fontSize: 8, fontWeight: "900", width: 42 },
  eventTitle: { color: "#12100e", flex: 1, fontSize: 11, fontWeight: "900" },
  eventToggle: { color: "#12100e", fontSize: 15, fontWeight: "900", textAlign: "center", width: 18 },
  stepText: { color: "#6c665f", fontSize: 9, fontWeight: "900" },
  eventText: { color: "#12100e", fontSize: 11, lineHeight: 17, marginTop: 3 },
  errorText: { color: "#8a2119", fontSize: 11, lineHeight: 17, marginTop: 3 },
  details: { borderLeftColor: "#c9c0b3", borderLeftWidth: 1, gap: 7, marginTop: 6, paddingLeft: 8 },
  detailSection: { gap: 3 },
  detailLabel: { color: "#6c665f", fontSize: 8, fontWeight: "900", textTransform: "uppercase" },
  codeText: { color: "#12100e", fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 10, lineHeight: 15 },
  errorCodeText: { color: "#8a2119", fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 10, lineHeight: 15 },
  truncatedText: { color: "#8a4a00", fontSize: 9, fontWeight: "900" },
  runError: { backgroundColor: "#ffb1a3", borderColor: "#12100e", borderRadius: 5, borderWidth: 2, color: "#6d1812", fontSize: 10, marginTop: 4, padding: 7 },
});
