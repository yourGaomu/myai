import type { RefObject } from "react";
import { ActivityIndicator, ScrollView, StyleSheet, Text, View } from "react-native";

import type { ChatItem } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { AssistantLoadingBubble } from "./AssistantLoadingBubble";
import { MessageBubble } from "./MessageBubble";

type Props = {
  buttonFeedback: ButtonFeedback;
  chatScrollRef: RefObject<ScrollView | null>;
  height: number;
  loadingHistory: boolean;
  messages: ChatItem[];
  showAssistantLoading: boolean;
};

export function ChatPanel({
  buttonFeedback,
  chatScrollRef,
  height,
  loadingHistory,
  messages,
  showAssistantLoading,
}: Props) {
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
          messages.map((message) => (
            <MessageBubble key={message.id} buttonFeedback={buttonFeedback} message={message} />
          ))
        )}
        {showAssistantLoading ? <AssistantLoadingBubble /> : null}
      </ScrollView>
    </View>
  );
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
