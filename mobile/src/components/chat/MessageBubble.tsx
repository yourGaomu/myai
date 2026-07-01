import { useState } from "react";
import { Pressable, Text, type StyleProp, View, type ViewStyle } from "react-native";

import type { ChatItem } from "../../types/chat";
import { usageHasValues, usageSummary } from "../../utils/tokenUsage";
import { MarkdownText } from "./MarkdownText";
import { styles } from "./styles";

type Props = {
  message: ChatItem;
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
  onRegenerate: () => void;
};

export function MessageBubble({ message, buttonFeedback, onRegenerate }: Props) {
  const [expanded, setExpanded] = useState(message.role !== "tool_call" && message.role !== "tool");
  const canRegenerate = message.role === "assistant" && (message.status === "paused" || message.status === "error");
  const statusText = assistantStatusText(message);

  if (message.role === "tool_call" || message.role === "tool") {
    return (
      <View style={[styles.message, styles.toolMessage]}>
        <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => buttonFeedback(styles.toolHeader, pressed)}>
          <Text style={styles.toolBadge}>{message.role === "tool_call" ? "CALL" : message.toolError ? "ERR" : "DONE"}</Text>
          <View style={styles.flex}>
            <Text style={styles.toolTitle}>{message.toolName || "tool"}</Text>
            <Text style={styles.toolSubtitle}>{message.role === "tool_call" ? "Tool request" : "Tool result"}</Text>
          </View>
          <Text style={styles.toolToggle}>{expanded ? "Hide" : "Show"}</Text>
        </Pressable>
        {expanded ? (
          <View style={styles.toolBody}>
            {message.toolArguments ? (
              <View style={styles.toolSection}>
                <Text style={styles.toolSectionTitle}>Arguments</Text>
                <Text style={styles.toolCode}>{message.toolArguments}</Text>
              </View>
            ) : null}
            {message.text ? (
              <View style={styles.toolSection}>
                <Text style={styles.toolSectionTitle}>{message.toolError ? "Error" : "Result"}</Text>
                <Text style={[styles.toolCode, message.toolError && styles.toolErrorText]}>{message.text}</Text>
              </View>
            ) : null}
          </View>
        ) : null}
      </View>
    );
  }

  return (
    <View style={[styles.message, styles[`${message.role}Message`]]}>
      {message.reasoning ? (
        <View style={styles.reasoningBox}>
          <Text style={styles.reasoningTitle}>Thinking</Text>
          <Text style={styles.reasoningText}>{message.reasoning}</Text>
        </View>
      ) : null}
      <MarkdownText text={message.text} />
      {message.role === "assistant" && (statusText || canRegenerate) ? (
        <View style={styles.messageStatusRow}>
          {statusText ? <Text style={styles.messageStatusPill}>{statusText}</Text> : null}
          {canRegenerate ? (
            <Pressable onPress={onRegenerate} style={({ pressed }) => buttonFeedback(styles.regenerateButton, pressed)}>
              <Text style={styles.regenerateButtonText}>Regenerate</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}
      {message.usage && usageHasValues(message.usage) ? (
        <Text style={styles.messageMeta}>{usageSummary(message.usage)}</Text>
      ) : null}
    </View>
  );
}

function assistantStatusText(message: ChatItem) {
  switch (message.status) {
    case "streaming":
      return "Generating";
    case "tool_running":
      return "Using tool";
    case "paused":
      return "Paused";
    case "error":
      return "Failed";
    default:
      return "";
  }
}
