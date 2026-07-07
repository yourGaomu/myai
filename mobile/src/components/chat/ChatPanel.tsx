import { useCallback, useMemo, useRef, useState, type RefObject } from "react";
import { ActivityIndicator, ScrollView, StyleSheet, Text, View, type LayoutChangeEvent } from "react-native";

import type { ChatItem } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { groupToolActivity } from "../../utils/chatRenderItems";
import { AssistantLoadingBubble } from "./AssistantLoadingBubble";
import { ChatJumpNav, type ChatJumpAnchor } from "./ChatJumpNav";
import { MessageBubble } from "./MessageBubble";
import { ToolActivityGroup } from "./ToolActivityGroup";

type Props = {
  buttonFeedback: ButtonFeedback;
  chatScrollRef: RefObject<ScrollView | null>;
  height: number;
  loadingHistory: boolean;
  messages: ChatItem[];
  onRegenerate: () => void;
  showAssistantLoading: boolean;
};

export function ChatPanel({
  buttonFeedback,
  chatScrollRef,
  height,
  loadingHistory,
  messages,
  onRegenerate,
  showAssistantLoading,
}: Props) {
  const renderItems = useMemo(() => groupToolActivity(messages), [messages]);
  const [jumpOpen, setJumpOpen] = useState(false);
  const itemOffsetsRef = useRef<Record<string, number>>({});
  const jumpAnchors = useMemo(() => userMessageAnchors(messages), [messages]);

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
    <View style={[styles.panel, styles.chatPanel, { height }]}>
      <View style={styles.panelHeader}>
        <Text style={styles.panelTitle}>Chat</Text>
        <Text style={styles.pathText}>{messages.length} message(s)</Text>
      </View>
      <ScrollView
        contentContainerStyle={styles.messages}
        keyboardShouldPersistTaps="handled"
        onContentSizeChange={() => chatScrollRef.current?.scrollToEnd({ animated: true })}
        ref={chatScrollRef}
        showsVerticalScrollIndicator={false}
        style={styles.messagesScroll}
      >
        {loadingHistory ? (
          <View style={styles.inlineLoading}>
            <ActivityIndicator color="#12100e" size="small" />
            <Text style={styles.inlineLoadingText}>Loading session history...</Text>
          </View>
        ) : messages.length === 0 ? (
          <Text style={styles.emptyText}>Messages will appear here.</Text>
        ) : (
          renderItems.map((item) =>
            item.type === "tool_group" ? (
              <View key={item.id} onLayout={(event) => rememberItemOffset(item.id, event)}>
                <ToolActivityGroup buttonFeedback={buttonFeedback} group={item.group} />
              </View>
            ) : (
              <View key={item.id} onLayout={(event) => rememberItemOffset(item.message.id, event)}>
                <MessageBubble buttonFeedback={buttonFeedback} message={item.message} onRegenerate={onRegenerate} />
              </View>
            ),
          )
        )}
        {showAssistantLoading ? <AssistantLoadingBubble /> : null}
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
