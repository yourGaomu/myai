import { Pressable, StyleSheet, Text, View } from "react-native";

import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  buttonFeedback: ButtonFeedback;
  changesActive: boolean;
  onChangesPress: () => void;
  onChatPress: () => void;
  onFilesPress: () => void;
  onSessionsPress: () => void;
  onSettingsPress: () => void;
  viewMode: ViewMode;
};

export function BottomTabs({
  buttonFeedback,
  changesActive,
  onChangesPress,
  onChatPress,
  onFilesPress,
  onSessionsPress,
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
      <Pressable onPress={onSessionsPress} style={({ pressed }) => buttonFeedback([styles.segment, viewMode === "sessions" && styles.segmentActive], pressed)}>
        <Text style={[styles.segmentText, viewMode === "sessions" && styles.segmentTextActive]}>会话</Text>
      </Pressable>
      <Pressable onPress={onSettingsPress} style={({ pressed }) => buttonFeedback([styles.segment, viewMode === "settings" && styles.segmentActive], pressed)}>
        <Text style={[styles.segmentIconText, viewMode === "settings" && styles.segmentTextActive]}>⚙</Text>
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
    minHeight: 38,
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
  segmentIconText: {
    color: "#6c665f",
    fontSize: 16,
    fontWeight: "900",
    lineHeight: 18,
  },
  segmentTextActive: {
    color: "#12100e",
  },
});
