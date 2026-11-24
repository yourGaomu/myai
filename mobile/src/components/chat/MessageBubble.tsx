import { useState } from "react";
import { Pressable, StyleSheet, Text, View, type StyleProp, type ViewStyle } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, radius } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import type { AssetSummary } from "../../protocol";
import type { ChatItem } from "../../types/chat";
import { isImageAttachment } from "../../utils/attachments";
import { copyToClipboard } from "../../utils/clipboard";
import { assetSummaryToSharedAsset, isAssistantAssetTool, mergeSharedAssets, parseSharedAssets } from "../../utils/toolAssets";
import { toolIcon, toolKind } from "../../utils/toolDisplay";
import { usageHasValues } from "../../utils/tokenUsage";
import { TurnTokenUsage } from "./TurnTokenUsage";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { AssistantReply } from "./AssistantReply";
import { ChatAttachmentCard } from "./ChatAttachmentCard";
import { SharedAssetCard } from "./SharedAssetCard";
import { useChatStyles } from "./styles";
import { ThinkingReasoning } from "./ThinkingReasoning";

type Props = {
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
  catalog?: AssetSummary[];
  hideReasoning?: boolean;
  message: ChatItem;
  onRegenerate: () => void;
  sessionID?: string;
};

export function MessageBubble({ buttonFeedback, catalog, hideReasoning = false, message, onRegenerate, sessionID }: Props) {
  const { palette } = useAppTheme();
  const styles = useChatStyles();
  const local = useThemedStyles(makeLocalStyles);
  const sharedAssets = message.role === "tool" ? mergeSharedAssets(parseSharedAssets(message.toolName, message.text)) : [];
  const [expanded, setExpanded] = useState(message.role !== "tool_call" && (message.role !== "tool" || sharedAssets.length > 0));
  const [copied, setCopied] = useState(false);

  const canRegenerate = message.role === "assistant" && (message.status === "paused" || message.status === "error");
  const statusText = assistantStatusText(message);
  const showStatusText = statusText && message.status !== "streaming" && message.status !== "tool_running";
  const isStreaming = message.status === "streaming" || message.status === "tool_running";

  const handleCopyText = async (raw?: string) => {
    const content = (raw || "").trim();
    if (!content) return;
    if (await copyToClipboard(content)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    }
  };

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
                {sharedAssets.length > 0 && !failed ? (
                  sharedAssets.map((asset) => <SharedAssetCard key={asset.assetID} asset={asset} buttonFeedback={buttonFeedback} sessionID={sessionID} />)
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
    const hasImage = message.attachments?.some(isImageAttachment);
    return (
      <View style={[styles.userTurnWrapper, hasImage && styles.userImageTurnWrapper]}>
        <Pressable
          delayLongPress={280}
          onLongPress={() => void handleCopyText(message.text)}
          style={[styles.userMessage, hasImage && styles.userImageMessage]}
        >
          {message.text ? (
            <Text selectable style={styles.userMessageText}>
              {message.text}
            </Text>
          ) : null}
          {message.attachments?.length ? (
            <View style={styles.messageAttachments}>
              {message.attachments.map((attachment, index) => (
                <ChatAttachmentCard attachment={attachment} buttonFeedback={buttonFeedback} key={`${message.id}-attachment-${index}`} sessionID={sessionID} />
              ))}
            </View>
          ) : null}
        </Pressable>
        {message.status === "queued" || copied ? (
          <View style={styles.userTurnMetaRow}>
            {copied ? <Text style={[styles.userTurnMeta, { color: palette.ok }]}>已复制</Text> : null}
            {message.status === "queued" ? <Text style={styles.userTurnMeta}>排队中</Text> : null}
          </View>
        ) : null}
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
        <CharacterAvatar circular size={24} slot={isStreaming ? "working" : "idle"} variant="avatar" />
        <Text style={styles.assistantName}>MyAI</Text>
      </View>
      {message.reasoning && !hideReasoning ? (
        <ThinkingReasoning
          buttonFeedback={buttonFeedback}
          finishedAt={message.completedAt}
          reasoning={message.reasoning}
          running={isStreaming}
          startedAt={message.createdAt}
        />
      ) : null}
      <AssistantReply
        assets={message.assets?.filter((asset) => asset.asset_id && isAssistantAssetTool(asset.tool_name)).map(assetSummaryToSharedAsset)}
        buttonFeedback={buttonFeedback}
        catalog={catalog}
        sessionID={sessionID}
        text={message.text}
      />
      {message.attachments?.length ? (
        <View style={styles.messageAttachments}>
          {message.attachments.map((attachment, index) => (
            <ChatAttachmentCard attachment={attachment} buttonFeedback={buttonFeedback} key={`${message.id}-attachment-${index}`} sessionID={sessionID} />
          ))}
        </View>
      ) : null}
      {!isStreaming && (message.text || showStatusText || canRegenerate) ? (
        <View style={styles.messageStatusRow}>
          {showStatusText ? <Text style={styles.messageStatusPill}>{statusText}。</Text> : null}
          {message.text ? (
            <Pressable
              accessibilityLabel="复制回答"
              hitSlop={8}
              onPress={() => void handleCopyText(message.text)}
              style={({ pressed }) => buttonFeedback(local.copyAction, pressed)}
            >
              <Feather color={copied ? palette.ok : palette.ink3} name={copied ? "check" : "copy"} size={15} />
            </Pressable>
          ) : null}
          {canRegenerate ? (
            <Pressable hitSlop={8} onPress={onRegenerate} style={({ pressed }) => buttonFeedback(styles.regenerateButton, pressed)}>
              <Text style={styles.regenerateButtonText}>重新生成</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}
      <TurnTokenUsage id={message.id} running={isStreaming} show={Boolean(message.usage && usageHasValues(message.usage))} usage={message.usage} />
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

const makeLocalStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    toolIcon: { alignItems: "center", borderRadius: radius.xs + 2, height: 27, justifyContent: "center", width: 27 },
    toolIconError: { backgroundColor: palette.errWash },
    noticeRow: { alignItems: "flex-start", flexDirection: "row", gap: 8 },
    noticeIcon: { marginTop: 2 },
    noticeText: { color: palette.ink2, flex: 1, fontSize: font.label + 0.5, lineHeight: 21 },
    noticeTextError: { color: palette.err },
    copyAction: {
      alignItems: "center",
      borderRadius: radius.pill,
      height: 30,
      justifyContent: "center",
      width: 30,
    },
  });
