import { useEffect, useMemo, useState } from "react";
import { Platform, Pressable, StyleSheet, Text, View } from "react-native";

import type { AgentRunEvent, AgentRunSnapshot } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";

type ReasoningMode = "compact" | "raw" | "hidden";

type Props = {
  buttonFeedback: ButtonFeedback;
  snapshot: AgentRunSnapshot;
};

export function AgentRunTimeline({ buttonFeedback, snapshot }: Props) {
  const [expanded, setExpanded] = useState(snapshot.run.status === "running");
  const [reasoningMode, setReasoningMode] = useState<ReasoningMode>("compact");
  const elapsed = useRunElapsed(snapshot);
  const events = useMemo(() => {
    const visible = snapshot.events.filter((event) => reasoningMode !== "hidden" || event.type !== "reasoning");
    if (snapshot.run.status !== "running" && !visible.some(isTerminalEvent)) {
      visible.push(terminalEventFromRun(snapshot));
    }
    return visible;
  }, [reasoningMode, snapshot]);
  const hasReasoning = snapshot.events.some((event) => event.type === "reasoning");

  useEffect(() => {
    if (snapshot.run.status === "running") {
      setExpanded(true);
    }
  }, [snapshot.run.status]);

  return (
    <View style={styles.container}>
      <Pressable
        accessibilityLabel={expanded ? "Collapse agent run" : "Expand agent run"}
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
          {hasReasoning ? (
            <View style={styles.modeRow}>
              <Text style={styles.modeLabel}>Reasoning</Text>
              {(["compact", "raw", "hidden"] as ReasoningMode[]).map((mode) => (
                <Pressable
                  accessibilityLabel={`${mode} reasoning`}
                  key={mode}
                  onPress={() => setReasoningMode(mode)}
                  style={({ pressed }) => buttonFeedback([
                    styles.modeButton,
                    reasoningMode === mode && styles.modeButtonActive,
                  ], pressed)}
                >
                  <Text style={[styles.modeButtonText, reasoningMode === mode && styles.modeButtonTextActive]}>
                    {modeLabel(mode)}
                  </Text>
                </Pressable>
              ))}
            </View>
          ) : null}

          {events.length === 0 ? (
            <Text style={styles.emptyText}>Waiting for activity...</Text>
          ) : (
            events.map((event, index) => (
              <TimelineEvent
                event={event}
                key={`${event.id}-${event.sequence}`}
                last={index === events.length - 1}
                reasoningMode={reasoningMode}
              />
            ))
          )}
          {snapshot.run.error_message ? <Text style={styles.runError}>{snapshot.run.error_message}</Text> : null}
        </View>
      ) : null}
    </View>
  );
}

function TimelineEvent({ event, last, reasoningMode }: { event: AgentRunEvent; last: boolean; reasoningMode: ReasoningMode }) {
  const expandable = event.type === "tool_call" || event.type === "tool_result" || event.type === "permission";
  const [expanded, setExpanded] = useState(event.type === "tool_result" && isFailure(event.status));
  const content = eventContent(event);
  const compactReasoning = event.type === "reasoning" && reasoningMode === "compact";

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

        {event.type === "reasoning" && content ? (
          <Text numberOfLines={compactReasoning ? 4 : undefined} style={styles.reasoningText}>{content}</Text>
        ) : null}
        {event.type === "plan_update" && content ? <Text style={styles.eventText}>{content}</Text> : null}
        {event.type === "progress" && content ? <Text style={styles.eventText}>{content}</Text> : null}
        {isTerminalEvent(event) && content ? <Text style={isFailure(event.status) ? styles.errorText : styles.eventText}>{content}</Text> : null}

        {expandable && expanded ? (
          <View style={styles.details}>
            {event.arguments ? (
              <View style={styles.detailSection}>
                <Text style={styles.detailLabel}>Arguments</Text>
                <Text style={styles.codeText}>{event.arguments}</Text>
              </View>
            ) : null}
            {content ? (
              <View style={styles.detailSection}>
                <Text style={styles.detailLabel}>{isFailure(event.status) ? "Error" : "Result"}</Text>
                <Text style={isFailure(event.status) ? styles.errorCodeText : styles.codeText}>{content}</Text>
              </View>
            ) : null}
            {event.truncated ? <Text style={styles.truncatedText}>Output truncated</Text> : null}
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
    return "0s";
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
    title: type === "completed" ? "Run completed" : `Run ${type}`,
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
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}

function runMeta(snapshot: AgentRunSnapshot) {
  const { run } = snapshot;
  const status = run.status === "succeeded" ? "complete" : run.status;
  if (run.total_steps) {
    return `${status} / step ${run.current_step || 0} of ${run.total_steps}`;
  }
  return `${status} / ${snapshot.events.length} event${snapshot.events.length === 1 ? "" : "s"}`;
}

function runKindLabel(kind: string) {
  switch (kind) {
    case "plan": return "Plan execution";
    case "regenerate": return "Regenerate response";
    case "internal": return "Agent task";
    default: return "Agent run";
  }
}

function modeLabel(mode: ReasoningMode) {
  switch (mode) {
    case "raw": return "Raw";
    case "hidden": return "Hide";
    default: return "Fold";
  }
}

function eventLabel(event: AgentRunEvent) {
  switch (event.type) {
    case "reasoning": return "THINK";
    case "tool_call": return "TOOL";
    case "tool_result": return isFailure(event.status) ? "ERROR" : "RESULT";
    case "permission": return "ACCESS";
    case "plan_update": return "PLAN";
    case "failed": return "FAILED";
    case "paused": return "PAUSED";
    case "canceled": return "STOP";
    case "completed": return "DONE";
    default: return "STEP";
  }
}

function eventTitle(event: AgentRunEvent) {
  if (event.tool_name) {
    return event.tool_name;
  }
  return event.title || event.type.replace(/_/g, " ");
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
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    overflow: "hidden",
  },
  header: {
    alignItems: "center",
    flexDirection: "row",
    gap: 9,
    minHeight: 52,
    paddingHorizontal: 10,
    paddingVertical: 8,
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
  body: { borderTopColor: "#12100e", borderTopWidth: 3, paddingHorizontal: 10, paddingVertical: 9 },
  modeRow: { alignItems: "center", flexDirection: "row", gap: 5, marginBottom: 10 },
  modeLabel: { color: "#6c665f", flex: 1, fontSize: 10, fontWeight: "900", textTransform: "uppercase" },
  modeButton: { borderColor: "#12100e", borderRadius: 5, borderWidth: 2, paddingHorizontal: 7, paddingVertical: 4 },
  modeButtonActive: { backgroundColor: "#12100e" },
  modeButtonText: { color: "#12100e", fontSize: 9, fontWeight: "900" },
  modeButtonTextActive: { color: "#fffaf0" },
  emptyText: { color: "#6c665f", fontSize: 11, paddingVertical: 5 },
  eventRow: { flexDirection: "row", minHeight: 38 },
  rail: { alignItems: "center", marginRight: 8, width: 14 },
  eventDot: { borderColor: "#12100e", borderRadius: 6, borderWidth: 2, height: 12, marginTop: 4, width: 12, zIndex: 1 },
  railLine: { backgroundColor: "#12100e", flex: 1, marginBottom: -4, marginTop: -1, width: 2 },
  eventContent: { flex: 1, minWidth: 0, paddingBottom: 11 },
  eventHeader: { alignItems: "center", flexDirection: "row", gap: 6, minHeight: 22 },
  eventBadge: { color: "#6c665f", fontSize: 8, fontWeight: "900", width: 42 },
  eventTitle: { color: "#12100e", flex: 1, fontSize: 11, fontWeight: "900" },
  eventToggle: { color: "#12100e", fontSize: 15, fontWeight: "900", textAlign: "center", width: 18 },
  stepText: { color: "#6c665f", fontSize: 9, fontWeight: "900" },
  reasoningText: { color: "#3f3748", fontSize: 11, lineHeight: 17, marginTop: 3 },
  eventText: { color: "#12100e", fontSize: 11, lineHeight: 17, marginTop: 3 },
  errorText: { color: "#8a2119", fontSize: 11, lineHeight: 17, marginTop: 3 },
  details: { borderLeftColor: "#12100e", borderLeftWidth: 2, gap: 7, marginTop: 6, paddingLeft: 8 },
  detailSection: { gap: 3 },
  detailLabel: { color: "#6c665f", fontSize: 8, fontWeight: "900", textTransform: "uppercase" },
  codeText: { color: "#12100e", fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 10, lineHeight: 15 },
  errorCodeText: { color: "#8a2119", fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 10, lineHeight: 15 },
  truncatedText: { color: "#8a4a00", fontSize: 9, fontWeight: "900" },
  runError: { backgroundColor: "#ffb1a3", borderColor: "#12100e", borderRadius: 5, borderWidth: 2, color: "#6d1812", fontSize: 10, marginTop: 4, padding: 7 },
});
