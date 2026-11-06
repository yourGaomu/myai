import { useState } from "react";
import { Image, Pressable, StyleSheet, Text, View, type StyleProp, type ViewStyle } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, palette, radius } from "../../constants/theme";
import type { ChatItem } from "../../types/chat";
import { toolIcon, toolKind } from "../../utils/toolDisplay";
import { parseSharedAsset } from "../../utils/toolAssets";
import { usageHasValues, usageSummary } from "../../utils/tokenUsage";
import { MarkdownText } from "./MarkdownText";
import { ChatAttachmentCard } from "./ChatAttachmentCard";
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
    const isKnowledgeSearch = message.toolName === "knowledge_search";
    const displayName = isKnowledgeSearch ? "知识库检索" : message.toolName || "工具";
    const failed = message.role === "tool" && Boolean(message.toolError);
    const subtitle = message.role === "tool_call" ? "调用中" : failed ? "执行错误" : "已完成";
    return (
      <View style={[styles.message, styles.toolMessage]}>
        <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => buttonFeedback(styles.toolHeader, pressed)}>
          <View style={[local.toolIcon, failed && local.toolIconError]}>
            <Feather color={failed ? palette.err : palette.ink3} name={toolIcon(toolKind(message.toolName || ""))} size={15} />
          </View>
          <Text numberOfLines={1} style={styles.toolTitle}>
            {displayName}
          </Text>
          <Text numberOfLines={1} style={[styles.toolSubtitle, failed && styles.toolErrorText]}>
            {subtitle}
          </Text>
          <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={14} />
        </Pressable>
        {expanded ? (
          <View style={styles.toolBody}>
            {message.toolArguments ? (
              <View style={styles.toolSection}>
                <Text style={styles.toolSectionTitle}>请求</Text>
                <Text selectable style={styles.toolCode}>{message.toolArguments}</Text>
              </View>
            ) : null}
            {message.text ? (
              <View style={styles.toolSection}>
                <Text style={[styles.toolSectionTitle, failed && styles.toolErrorText]}>{failed ? "错误" : "返回"}</Text>
                {sharedAsset && !failed ? (
                  <SharedAssetCard asset={sharedAsset} buttonFeedback={buttonFeedback} />
                ) : (
                  <Text selectable style={[styles.toolCode, failed && styles.toolErrorText]}>{message.text}</Text>
                )}
              </View>
            ) : null}
          </View>
        ) : null}
      </View>
    );
  }

  if (message.role === "user") {
    return (
      <View style={styles.userTurnWrapper}>
        <View style={styles.userMessage}>
          {message.text ? (
            <Text selectable style={styles.userMessageText}>
              {message.text}
            </Text>
          ) : null}
          {message.attachments?.length ? (
            <View style={styles.messageAttachments}>
              {message.attachments.map((attachment, index) => (
                <ChatAttachmentCard attachment={attachment} buttonFeedback={buttonFeedback} key={`${message.id}-attachment-${index}`} />
              ))}
            </View>
          ) : null}
        </View>
        {message.status === "queued" ? <Text style={styles.userTurnMeta}>排队中</Text> : null}
      </View>
    );
  }

  if (message.role === "event" || message.role === "error") {
    const isError = message.role === "error";
    return (
      <View style={[styles.message, isError ? styles.errorMessage : styles.eventMessage, local.noticeRow]}>
        <Feather color={isError ? palette.err : palette.ink3} name={isError ? "alert-circle" : "info"} size={15} style={local.noticeIcon} />
        <Text selectable style={[local.noticeText, isError && local.noticeTextError]}>
          {message.text}
        </Text>
      </View>
    );
  }

  return (
    <View style={[styles.message, styles.assistantMessage]}>
      <View style={styles.assistantHeader}>
        <Image source={require("../../../assets/avatar.png")} style={styles.assistantAvatarImage} />
        <Text style={styles.assistantName}>MyAI</Text>
      </View>
      {message.reasoning && !hideReasoning ? (
        <ThinkingReasoning
          buttonFeedback={buttonFeedback}
          finishedAt={message.completedAt}
          reasoning={message.reasoning}
          running={message.status === "streaming" || message.status === "tool_running"}
          startedAt={message.createdAt}
        />
      ) : null}
      {message.text ? <MarkdownText text={message.text} /> : null}
      {message.attachments?.length ? (
        <View style={styles.messageAttachments}>
          {message.attachments.map((attachment, index) => (
            <ChatAttachmentCard attachment={attachment} buttonFeedback={buttonFeedback} key={`${message.id}-attachment-${index}`} />
          ))}
        </View>
      ) : null}
      {showStatusText || canRegenerate ? (
        <View style={styles.messageStatusRow}>
          {showStatusText ? <Text style={styles.messageStatusPill}>{statusText}。</Text> : null}
          {canRegenerate ? (
            <Pressable hitSlop={8} onPress={onRegenerate} style={({ pressed }) => buttonFeedback(styles.regenerateButton, pressed)}>
              <Text style={styles.regenerateButtonText}>重新生成</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}
      {message.usage && usageHasValues(message.usage) ? <Text style={styles.messageMeta}>{usageSummary(message.usage)}</Text> : null}
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
      return "已停止";
    case "error":
      return "失败";
    default:
      return "";
  }
}

const local = StyleSheet.create({
  toolIcon: { alignItems: "center", borderRadius: radius.xs + 2, height: 27, justifyContent: "center", width: 27 },
  toolIconError: { backgroundColor: palette.errWash },
  noticeRow: { alignItems: "flex-start", flexDirection: "row", gap: 8 },
  noticeIcon: { marginTop: 2 },
  noticeText: { color: palette.ink2, flex: 1, fontSize: font.label + 0.5, lineHeight: 21 },
  noticeTextError: { color: palette.err },
});
