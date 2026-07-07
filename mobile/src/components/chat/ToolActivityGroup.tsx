import { useMemo, useState } from "react";
import { Pressable, Text, View } from "react-native";

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
        <Text style={[styles.toolGroupBadge, group.failedCount > 0 && styles.toolGroupBadgeError]}>{group.failedCount > 0 ? "ERR" : "TOOLS"}</Text>
        <View style={styles.flex}>
          <Text style={styles.toolGroupTitle}>Tool activity</Text>
          <Text numberOfLines={1} style={styles.toolGroupSubtitle}>
            {summary}
          </Text>
        </View>
        <Text style={styles.toolToggle}>{expanded ? "Hide" : "Show"}</Text>
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
          <Text style={styles.toolGroupStepBadge}>OK</Text>
          <Text style={styles.toolGroupStepTitle}>Permission</Text>
          <Text style={styles.toolGroupStepMeta}>#{index}</Text>
        </View>
        <Text style={styles.toolGroupStepText}>{message.text}</Text>
      </View>
    );
  }

  const sharedAsset = parseSharedAsset(message.toolName, message.text);
  const failed = Boolean(message.toolError);
  const badge = message.role === "tool_call" ? "CALL" : failed ? "ERR" : "DONE";

  return (
    <View style={styles.toolGroupStep}>
      <View style={styles.toolGroupStepHeader}>
        <Text style={[styles.toolGroupStepBadge, failed && styles.toolGroupStepBadgeError]}>{badge}</Text>
        <Text numberOfLines={1} style={styles.toolGroupStepTitle}>
          {message.toolName || "tool"}
        </Text>
        <Text style={styles.toolGroupStepMeta}>#{index}</Text>
      </View>
      {message.toolArguments ? (
        <View style={styles.toolGroupStepSection}>
          <Text style={styles.toolGroupStepLabel}>Arguments</Text>
          <Text numberOfLines={6} selectable style={styles.toolGroupCode}>
            {message.toolArguments}
          </Text>
        </View>
      ) : null}
      {message.text ? (
        <View style={styles.toolGroupStepSection}>
          <Text style={styles.toolGroupStepLabel}>{failed ? "Error" : "Result"}</Text>
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
  const parts = [`${group.messages.length} step(s)`];
  if (group.callCount > 0) {
    parts.push(`${group.callCount} call(s)`);
  }
  if (group.resultCount > 0) {
    parts.push(`${group.resultCount} result(s)`);
  }
  if (group.permissionCount > 0) {
    parts.push(`${group.permissionCount} permission event(s)`);
  }
  if (group.assetCount > 0) {
    parts.push(`${group.assetCount} asset(s)`);
  }
  if (group.failedCount > 0) {
    parts.push(`${group.failedCount} failed`);
  }

  const names = group.names.slice(0, 4).join(", ");
  if (names) {
    parts.push(names + (group.names.length > 4 ? "..." : ""));
  }
  return parts.join(" / ");
}
