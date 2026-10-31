import { useMemo, useState } from "react";
import { Pressable, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { palette } from "../../constants/theme";
import type { ChatItem } from "../../types/chat";
import type { ButtonFeedback } from "../../types/ui";
import { isPermissionEvent, type ToolActivityGroupItem } from "../../utils/chatRenderItems";
import { parseSharedAsset } from "../../utils/toolAssets";
import { SharedAssetCard } from "./SharedAssetCard";
import { styles } from "./styles";

type Props = {
  buttonFeedback: ButtonFeedback;
  group: ToolActivityGroupItem;
};

export function ToolActivityGroup({ buttonFeedback, group }: Props) {
  const [expanded, setExpanded] = useState(false);
  const summary = useMemo(() => toolActivitySummary(group), [group]);

  return (
    <View style={[styles.message, styles.toolGroupMessage]}>
      <Pressable onPress={() => setExpanded((value) => !value)} style={({ pressed }) => buttonFeedback(styles.toolGroupHeader, pressed)}>
        <View style={[styles.toolDot, group.failedCount > 0 ? styles.toolDotErr : styles.toolDotOk]} />
        <Text style={styles.toolGroupTitle}>工具活动</Text>
        <Text numberOfLines={1} style={styles.toolGroupSubtitle}>
          {summary}
        </Text>
        <Feather color={palette.ink4} name={expanded ? "chevron-down" : "chevron-right"} size={13} />
      </Pressable>
      {expanded ? (
        <View style={styles.toolGroupBody}>
          {group.messages.map((message, index) => (
            <ToolActivityStep key={message.id} buttonFeedback={buttonFeedback} index={index + 1} message={message} />
          ))}
        </View>
      ) : null}
    </View>
  );
}

function ToolActivityStep({ buttonFeedback, index, message }: { buttonFeedback: ButtonFeedback; index: number; message: ChatItem }) {
  if (isPermissionEvent(message)) {
    return (
      <View style={styles.toolGroupStep}>
        <View style={styles.toolGroupStepHeader}>
          <View style={[styles.toolDot, styles.toolDotOk]} />
          <Text style={styles.toolGroupStepTitle}>权限</Text>
          <Text style={styles.toolGroupStepMeta}>#{index}</Text>
        </View>
        <Text style={styles.toolGroupStepText}>{message.text}</Text>
      </View>
    );
  }

  const sharedAsset = parseSharedAsset(message.toolName, message.text);
  const failed = Boolean(message.toolError);
  const dotStyle = message.role === "tool_call" ? styles.toolDotWarn : failed ? styles.toolDotErr : styles.toolDotOk;

  return (
    <View style={styles.toolGroupStep}>
      <View style={styles.toolGroupStepHeader}>
        <View style={[styles.toolDot, dotStyle]} />
        <Text numberOfLines={1} style={styles.toolGroupStepTitle}>
          {message.toolName === "knowledge_search" ? "知识库检索" : message.toolName || "tool"}
        </Text>
        <Text style={styles.toolGroupStepMeta}>#{index}</Text>
      </View>
      {message.role === "tool_call" && message.toolArguments ? (
        <View style={styles.toolGroupStepSection}>
          <Text style={styles.toolGroupStepLabel}>参数</Text>
          <Text numberOfLines={6} selectable style={styles.toolGroupCode}>
            {message.toolArguments}
          </Text>
        </View>
      ) : null}
      {message.text ? (
        <View style={styles.toolGroupStepSection}>
          <Text style={styles.toolGroupStepLabel}>{failed ? "错误" : "结果"}</Text>
          {sharedAsset && !failed ? (
            <SharedAssetCard asset={sharedAsset} buttonFeedback={buttonFeedback} />
          ) : (
            <Text numberOfLines={8} selectable style={[styles.toolGroupCode, failed && styles.toolErrorText]}>
              {message.text}
            </Text>
          )}
        </View>
      ) : null}
    </View>
  );
}

function toolActivitySummary(group: ToolActivityGroupItem) {
  const parts = [`${group.messages.length} 个步骤`];
  if (group.callCount > 0) {
    parts.push(`${group.callCount} 次调用`);
  }
  if (group.resultCount > 0) {
    parts.push(`${group.resultCount} 个结果`);
  }
  if (group.permissionCount > 0) {
    parts.push(`${group.permissionCount} 个权限事件`);
  }
  if (group.assetCount > 0) {
    parts.push(`${group.assetCount} 个资源`);
  }
  if (group.failedCount > 0) {
    parts.push(`${group.failedCount} 个失败`);
  }

  const names = group.names
    .map((name) => (name === "knowledge_search" ? "知识库检索" : name))
    .slice(0, 4)
    .join(", ");
  if (names) {
    parts.push(names + (group.names.length > 4 ? "..." : ""));
  }
  return parts.join(" / ");
}
