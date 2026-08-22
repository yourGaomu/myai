import { Pressable, StyleSheet, Text, View } from "react-native";

import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  buttonFeedback: ButtonFeedback;
  changesActive: boolean;
  onChangesPress: () => void;
  onChatPress: () => void;
  onFilesPress: () => void;
  onKnowledgePress: () => void;
  onSettingsPress: () => void;
  viewMode: ViewMode;
};

export function BottomTabs({
  buttonFeedback,
  changesActive,
  onChangesPress,
  onChatPress,
  onFilesPress,
  onKnowledgePress,
  onSettingsPress,
  viewMode,
}: Props) {
  return (
    <View style={styles.segmented}>
      <Pressable onPress={onChatPress} style={({ pressed }) => buttonFeedback([styles.segment, viewMode === "chat" && styles.segmentActive], pressed)}>
        <Text style={[styles.segmentText, viewMode === "chat" && styles.segmentTextActive]}>对话</Text>
      </Pressable>
      <Pressable onPress={onFilesPress} style={({ pressed }) => buttonFeedback([styles.segment, viewMode === "files" && styles.segmentActive], pressed)}>
        <Text style={[styles.segmentText, viewMode === "files" && styles.segmentTextActive]}>文件</Text>
      </Pressable>
      <Pressable onPress={onChangesPress} style={({ pressed }) => buttonFeedback([styles.segment, changesActive && styles.segmentActive], pressed)}>
        <Text style={[styles.segmentText, changesActive && styles.segmentTextActive]}>变更</Text>
      </Pressable>
      <Pressable onPress={onKnowledgePress} style={({ pressed }) => buttonFeedback([styles.segment, viewMode === "knowledge" && styles.segmentActive], pressed)}>
        <Text style={[styles.segmentText, viewMode === "knowledge" && styles.segmentTextActive]}>知识</Text>
      </Pressable>
      <Pressable
        accessibilityLabel="打开设置"
        accessibilityRole="tab"
        onPress={onSettingsPress}
        style={({ pressed }) => buttonFeedback([styles.segment, viewMode === "settings" && styles.segmentActive], pressed)}
      >
        <Text style={[styles.segmentText, viewMode === "settings" && styles.segmentTextActive]}>设置</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  segmented: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    padding: 4,
  },
  segment: {
    alignItems: "center",
    borderRadius: 6,
    flex: 1,
    justifyContent: "center",
    minHeight: 44,
    paddingVertical: 7,
  },
  segmentActive: {
    backgroundColor: "#ffd84f",
  },
  segmentText: {
    color: "#6c665f",
    fontSize: 13,
    fontWeight: "900",
  },
  segmentTextActive: {
    color: "#12100e",
  },
});
