import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import {
  ActivityIndicator,
  ScrollView,
  StyleSheet,
  Text,
  View,
  type LayoutChangeEvent,
} from "react-native";

import type { AgentRunSnapshot } from "../../protocol";
import type { ChatItem } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { groupToolActivity, type ChatRenderItem } from "../../utils/chatRenderItems";
import { AgentRunTimeline } from "./AgentRunTimeline";
import { AssistantLoadingBubble } from "./AssistantLoadingBubble";
import { ChatJumpNav, type ChatJumpAnchor } from "./ChatJumpNav";
import { MessageBubble } from "./MessageBubble";
import { ToolActivityGroup } from "./ToolActivityGroup";

type Props = {
  activeAssistantID: string;
  buttonFeedback: ButtonFeedback;
  chatScrollRef: RefObject<ScrollView | null>;
  loadingHistory: boolean;
  messages: ChatItem[];
  pendingRequestID: string;
  runs: AgentRunSnapshot[];
  sessionID: string;
  onRegenerate: () => void;
};

export function ChatPanel({
  activeAssistantID,
  buttonFeedback,
  chatScrollRef,
  loadingHistory,
  messages,
  pendingRequestID,
  runs,
  sessionID,
  onRegenerate,
}: Props) {
  const renderItems = useMemo(() => {
    const visibleMessages = messages.filter((message) => !isCoveredToolActivity(message, runs));
    return injectAgentRuns(groupToolActivity(visibleMessages), visibleMessages, runs);
  }, [messages, runs]);
  const [jumpOpen, setJumpOpen] = useState(false);
  const itemOffsetsRef = useRef<Record<string, number>>({});
  const followTailRef = useRef(true);
  const jumpAnchors = useMemo(() => userMessageAnchors(messages), [messages]);
  const assistantLoadingLabel = useMemo(
    () => loadingLabel(pendingRequestID, activeAssistantID, messages, runs),
    [activeAssistantID, messages, pendingRequestID, runs],
  );

  useEffect(() => {
    followTailRef.current = true;
    itemOffsetsRef.current = {};
  }, [sessionID]);

  const rememberItemOffset = useCallback((id: string, event: LayoutChangeEvent) => {
    itemOffsetsRef.current[id] = event.nativeEvent.layout.y;
  }, []);

  const jumpToMessage = useCallback(
    (id: string) => {
      const y = itemOffsetsRef.current[id] ?? 0;
      chatScrollRef.current?.scrollTo({ y: Math.max(0, y - 8), animated: true });
      setJumpOpen(false);
    },
    [chatScrollRef],
  );

  return (
    <View style={[styles.panel, styles.chatPanel]}>
      <View style={styles.panelHeader}>
        <Text style={styles.panelTitle}>Chat</Text>
        <Text style={styles.pathText}>{messages.length} message(s)</Text>
      </View>
      <ScrollView
        contentContainerStyle={styles.messages}
        keyboardShouldPersistTaps="handled"
        nestedScrollEnabled
        onContentSizeChange={() => {
          if (followTailRef.current) {
            chatScrollRef.current?.scrollToEnd({ animated: true });
          }
        }}
        onScroll={(event) => {
          const { contentOffset, contentSize, layoutMeasurement } = event.nativeEvent;
          const distanceFromBottom = contentSize.height - (contentOffset.y + layoutMeasurement.height);
          followTailRef.current = distanceFromBottom <= 36;
        }}
        scrollEventThrottle={100}
        ref={chatScrollRef}
        showsVerticalScrollIndicator={false}
        style={styles.messagesScroll}
      >
        {loadingHistory ? (
          <View style={styles.inlineLoading}>
            <ActivityIndicator color="#12100e" size="small" />
            <Text style={styles.inlineLoadingText}>Loading session history...</Text>
          </View>
        ) : messages.length === 0 && runs.length === 0 ? (
          <Text style={styles.emptyText}>Messages will appear here.</Text>
        ) : (
          renderItems.map((item) =>
            item.type === "run" ? (
              <View key={item.id} onLayout={(event) => rememberItemOffset(item.id, event)}>
                <AgentRunTimeline buttonFeedback={buttonFeedback} snapshot={item.snapshot} />
              </View>
            ) : item.type === "tool_group" ? (
              <View key={item.id} onLayout={(event) => rememberItemOffset(item.id, event)}>
                <ToolActivityGroup buttonFeedback={buttonFeedback} group={item.group} />
              </View>
            ) : (
              <View key={item.id} onLayout={(event) => rememberItemOffset(item.message.id, event)}>
                <MessageBubble
                  buttonFeedback={buttonFeedback}
                  hideReasoning={item.message.role === "assistant" && isCoveredByRun(item.message, runs)}
                  message={item.message}
                  onRegenerate={onRegenerate}
                />
              </View>
            ),
          )
        )}
        {pendingRequestID ? <AssistantLoadingBubble label={assistantLoadingLabel} /> : null}
      </ScrollView>
      <ChatJumpNav
        anchors={jumpAnchors}
        buttonFeedback={buttonFeedback}
        onJump={jumpToMessage}
        onToggle={() => setJumpOpen((value) => !value)}
        open={jumpOpen}
      />
    </View>
  );
}

type PanelRenderItem = ChatRenderItem | { type: "run"; id: string; snapshot: AgentRunSnapshot };

function injectAgentRuns(items: ChatRenderItem[], messages: ChatItem[], runs: AgentRunSnapshot[]): PanelRenderItem[] {
  if (runs.length === 0) {
    return items;
  }
  const result: PanelRenderItem[] = [];
  const inserted = new Set<string>();
  const users = messages.filter((message) => message.role === "user");

  items.forEach((item) => {
    result.push(item);
    if (item.type !== "message") {
      return;
    }
    const anchor = item.message;
    const nextUser = anchor.role === "user" ? users[users.indexOf(anchor) + 1] : undefined;
    runs.forEach((snapshot) => {
      if (inserted.has(snapshot.run.id)) {
        return;
      }
      const requestMatch = Boolean(anchor.requestID && snapshot.run.request_id === anchor.requestID);
      const timeMatch = anchor.role === "user" && runFollowsUser(snapshot, anchor, nextUser);
      if (requestMatch || timeMatch) {
        result.push({ type: "run", id: `agent-run-${snapshot.run.id}`, snapshot });
        inserted.add(snapshot.run.id);
      }
    });
  });

  runs.forEach((snapshot) => {
    if (!inserted.has(snapshot.run.id)) {
      result.push({ type: "run", id: `agent-run-${snapshot.run.id}`, snapshot });
    }
  });
  return result;
}

function runFollowsUser(snapshot: AgentRunSnapshot, user: ChatItem, nextUser?: ChatItem) {
  const startedAt = Date.parse(snapshot.run.started_at);
  const userAt = user.createdAt ? Date.parse(user.createdAt) : Number.NaN;
  const nextUserAt = nextUser?.createdAt ? Date.parse(nextUser.createdAt) : Number.POSITIVE_INFINITY;
  return Number.isFinite(startedAt) && Number.isFinite(userAt) && startedAt >= userAt - 1000 && startedAt < nextUserAt;
}

function isCoveredToolActivity(message: ChatItem, runs: AgentRunSnapshot[]) {
  const permissionEvent = message.role === "event" && /^(Allowed|Denied)\s+\S+/.test(message.text.trim());
  if (message.role !== "tool_call" && message.role !== "tool" && !permissionEvent) {
    return false;
  }
  return isCoveredByRun(message, runs);
}

function isCoveredByRun(message: ChatItem, runs: AgentRunSnapshot[]) {
  if (message.requestID && runs.some((snapshot) => snapshot.run.request_id === message.requestID)) {
    return true;
  }
  if (!message.createdAt) {
    return false;
  }
  const messageAt = Date.parse(message.createdAt);
  if (!Number.isFinite(messageAt)) {
    return false;
  }
  return runs.some(({ run }) => {
    const startedAt = Date.parse(run.started_at);
    const finishedAt = run.finished_at ? Date.parse(run.finished_at) : Date.now();
    return Number.isFinite(startedAt) && messageAt >= startedAt - 1000 && messageAt <= finishedAt + 30000;
  });
}

function loadingLabel(
  pendingRequestID: string,
  activeAssistantID: string,
  messages: ChatItem[],
  runs: AgentRunSnapshot[],
) {
  if (hasRunningTool(pendingRequestID, activeAssistantID, messages, runs)) {
    return "Running tool";
  }
  return activeAssistantID ? "Generating" : "Thinking";
}

function hasRunningTool(
  pendingRequestID: string,
  activeAssistantID: string,
  messages: ChatItem[],
  runs: AgentRunSnapshot[],
) {
  const matchingRun = [...runs]
    .reverse()
    .find(({ run }) => run.request_id === pendingRequestID && run.status === "running");
  if (matchingRun) {
    const lastToolCall = lastIndex(matchingRun.events, (event) => event.type === "tool_call");
    const lastToolResult = lastIndex(matchingRun.events, (event) => event.type === "tool_result");
    if (lastToolCall > lastToolResult) {
      return true;
    }
  }

  const requestMessages = messages.filter((message) => message.requestID === pendingRequestID);
  const lastToolCall = lastIndex(requestMessages, (message) => message.role === "tool_call");
  const lastToolResult = lastIndex(requestMessages, (message) => message.role === "tool");
  if (lastToolCall > lastToolResult) {
    return true;
  }

  return messages.some(
    (message) => message.id === activeAssistantID && message.status === "tool_running",
  );
}

function lastIndex<T>(items: T[], matches: (item: T) => boolean) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (matches(items[index])) {
      return index;
    }
  }
  return -1;
}

function userMessageAnchors(messages: ChatItem[]): ChatJumpAnchor[] {
  let userIndex = 0;
  return messages.reduce<ChatJumpAnchor[]>((anchors, message) => {
    if (message.role !== "user") {
      return anchors;
    }

    userIndex += 1;
    anchors.push({
      id: message.id,
      index: userIndex,
      title: messageTitle(message.text),
    });
    return anchors;
  }, []);
}

function messageTitle(text: string) {
  const normalized = text.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return "Empty message";
  }
  return normalized.length > 64 ? `${normalized.slice(0, 64)}...` : normalized;
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 10,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  chatPanel: {
    flex: 1,
    minHeight: 260,
    overflow: "hidden",
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  panelTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  pathText: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "700",
    marginTop: 3,
  },
  messagesScroll: {
    flex: 1,
  },
  messages: {
    gap: 8,
    paddingBottom: 4,
  },
  inlineLoading: {
    alignItems: "center",
    backgroundColor: "#fff4cc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 9,
  },
  inlineLoadingText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  emptyText: {
    color: "#6c665f",
  },
});
