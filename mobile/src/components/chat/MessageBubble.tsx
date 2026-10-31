import { useState } from "react";
import { Image, Pressable, Text, type StyleProp, View, type ViewStyle } from "react-native";
import { Feather } from "@expo/vector-icons";
import { LinearGradient } from "expo-linear-gradient";

import { palette } from "../../constants/theme";
import type { ChatItem } from "../../types/chat";
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
    const dotStyle =
      message.role === "tool_call"
        ? styles.toolDotWarn
        : message.toolError
          ? styles.toolDotErr
          : styles.toolDotOk;
    return (
      <View style={[styles.message, styles.toolMessage]}>
        <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => buttonFeedback(styles.toolHeader, pressed)}>
          <View style={[styles.toolDot, dotStyle]} />
          <Text numberOfLines={1} style={styles.toolTitle}>
            {displayName}
          </Text>
          <Text numberOfLines={1} style={styles.toolSubtitle}>
            {isKnowledgeSearch
              ? message.role === "tool_call"
                ? "知识库检索"
                : "知识库结果"
              : message.role === "tool_call"
                ? "工具请求"
                : message.toolError
                  ? "执行错误"
                  : "已完成"}
          </Text>
          <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={13} />
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

  if (message.role === "user") {
    return (
      <View style={styles.userTurnWrapper}>
        <LinearGradient
          colors={[palette.brandLight, palette.brandDeep]}
          end={{ x: 1, y: 1 }}
          start={{ x: 0.15, y: 0 }}
          style={[styles.message, styles.userMessage]}
        >
          {message.text ? (
            <Text selectable style={styles.userMessageText}>
              {message.text}
            </Text>
          ) : null}
          {message.attachments?.length ? (
            <View style={styles.messageAttachments}>
              {message.attachments.map((attachment, index) => (
                <ChatAttachmentCard
                  attachment={attachment}
                  buttonFeedback={buttonFeedback}
                  key={`${message.id}-attachment-${index}`}
                />
              ))}
            </View>
          ) : null}
        </LinearGradient>
        <Text style={styles.userTurnMeta}>{message.status === "queued" ? "排队中" : "已发送"}</Text>
      </View>
    );
  }

  return (
    <View style={[styles.message, styles[`${message.role}Message`]]}>
      {message.role === "assistant" ? (
        <View style={styles.assistantHeader}>
          <LinearGradient
            colors={[palette.brand, palette.brandDark]}
            end={{ x: 1, y: 1 }}
            start={{ x: 0.15, y: 0 }}
            style={styles.assistantAvatarRing}
          >
            <Image source={require("../../../assets/avatar.png")} style={styles.assistantAvatarImage} />
          </LinearGradient>
          <Text style={styles.assistantName}>MYAI</Text>
        </View>
      ) : null}
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
            <ChatAttachmentCard
              attachment={attachment}
              buttonFeedback={buttonFeedback}
              key={`${message.id}-attachment-${index}`}
            />
          ))}
        </View>
      ) : null}
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
