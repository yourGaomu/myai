import { useState } from "react";
import { Pressable, Text, type StyleProp, View, type ViewStyle } from "react-native";

import type { ChatItem } from "../../types/chat";
import { parseSharedAsset } from "../../utils/toolAssets";
import { usageHasValues, usageSummary } from "../../utils/tokenUsage";
import { MarkdownText } from "./MarkdownText";
import { SharedAssetCard } from "./SharedAssetCard";
import { styles } from "./styles";
import { ThinkingReasoning } from "./ThinkingReasoning";

type Props = {
  message: ChatItem;
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
  hideReasoning?: boolean;
  onRegenerate: () => void;
};

export function MessageBubble({ message, buttonFeedback, hideReasoning = false, onRegenerate }: Props) {
  const sharedAsset = parseSharedAsset(message.toolName, message.text);
  const [expanded, setExpanded] = useState(message.role !== "tool_call" && (message.role !== "tool" || Boolean(sharedAsset)));
  const canRegenerate = message.role === "assistant" && (message.status === "paused" || message.status === "error");
  const statusText = assistantStatusText(message);
  const showStatusText = statusText && message.status !== "streaming" && message.status !== "tool_running";

  if (message.role === "tool_call" || message.role === "tool") {
    return (
      <View style={[styles.message, styles.toolMessage]}>
        <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => buttonFeedback(styles.toolHeader, pressed)}>
          <Text style={styles.toolBadge}>{message.role === "tool_call" ? "调用" : message.toolError ? "错误" : "完成"}</Text>
          <View style={styles.flex}>
            <Text style={styles.toolTitle}>{message.toolName || "工具"}</Text>
            <Text style={styles.toolSubtitle}>{message.role === "tool_call" ? "工具请求" : "工具结果"}</Text>
          </View>
          <Text style={styles.toolToggle}>{expanded ? "收起" : "展开"}</Text>
        </Pressable>
        {expanded ? (
          <View style={styles.toolBody}>
            {message.toolArguments ? (
              <View style={styles.toolSection}>
                <Text style={styles.toolSectionTitle}>参数</Text>
                <Text style={styles.toolCode}>{message.toolArguments}</Text>
              </View>
            ) : null}
            {message.text ? (
              <View style={styles.toolSection}>
                <Text style={styles.toolSectionTitle}>{message.toolError ? "错误" : "结果"}</Text>
                {sharedAsset && !message.toolError ? (
                  <SharedAssetCard asset={sharedAsset} buttonFeedback={buttonFeedback} />
                ) : (
                  <Text style={[styles.toolCode, message.toolError && styles.toolErrorText]}>{message.text}</Text>
                )}
              </View>
            ) : null}
          </View>
        ) : null}
      </View>
    );
  }

  return (
    <View style={[styles.message, styles[`${message.role}Message`]]}>
      {message.reasoning && !hideReasoning ? (
        <ThinkingReasoning
          buttonFeedback={buttonFeedback}
          finishedAt={message.completedAt}
          reasoning={message.reasoning}
          running={message.status === "streaming" || message.status === "tool_running"}
          startedAt={message.createdAt}
        />
      ) : null}
      <MarkdownText text={message.text} />
      {message.role === "assistant" && (showStatusText || canRegenerate) ? (
        <View style={styles.messageStatusRow}>
          {showStatusText ? <Text style={styles.messageStatusPill}>{statusText}</Text> : null}
          {canRegenerate ? (
            <Pressable onPress={onRegenerate} style={({ pressed }) => buttonFeedback(styles.regenerateButton, pressed)}>
              <Text style={styles.regenerateButtonText}>重新生成</Text>
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
      return "正在生成";
    case "tool_running":
      return "正在使用工具";
    case "paused":
      return "已暂停";
    case "error":
      return "失败";
    default:
      return "";
  }
}
