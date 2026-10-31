import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";
import { BlurView } from "expo-blur";

import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { font, glass, palette, radius, shadow, space, weight } from "../../constants/theme";

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

type TabIcon = "message-circle" | "folder" | "git-branch" | "book-open" | "settings";

const TABS: { icon: TabIcon; label: string }[] = [
  { icon: "message-circle", label: "对话" },
  { icon: "folder", label: "文件" },
  { icon: "git-branch", label: "变更" },
  { icon: "book-open", label: "知识" },
  { icon: "settings", label: "设置" },
];

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
  // 与 TABS 顺序一一对应，避免在各处重复写判断
  const active = [
    viewMode === "chat",
    viewMode === "files",
    changesActive,
    viewMode === "knowledge",
    viewMode === "settings",
  ];
  const handlers = [onChatPress, onFilesPress, onChangesPress, onKnowledgePress, onSettingsPress];

  return (
    <BlurView intensity={glass.intensity} style={styles.tabs} tint="dark">
      {TABS.map((tab, index) => {
        const isActive = active[index];
        return (
          <Pressable
            accessibilityLabel={tab.label}
            accessibilityRole="tab"
            key={tab.label}
            onPress={handlers[index]}
            style={({ pressed }) =>
              buttonFeedback([styles.tab, isActive && styles.tabActive], pressed)
            }
          >
            <Feather
              color={isActive ? palette.brand : palette.ink4}
              name={tab.icon}
              size={19}
            />
            <Text style={[styles.tabText, isActive && styles.tabTextActive]}>{tab.label}</Text>
          </Pressable>
        );
      })}
    </BlurView>
  );
}

const styles = StyleSheet.create({
  tabs: {
    backgroundColor: palette.glassTintSoft,
    borderColor: palette.hairline,
    borderRadius: radius.lg,
    borderWidth: 1,
    flexDirection: "row",
    marginBottom: 22,
    marginHorizontal: space.lg,
    marginTop: space.md,
    overflow: "hidden",
    padding: 4,
    ...shadow.float,
  },
  tab: {
    alignItems: "center",
    borderRadius: radius.md,
    flex: 1,
    gap: 2,
    paddingBottom: 6,
    paddingTop: 5,
  },
  tabActive: {
    backgroundColor: palette.brandWash,
  },
  tabText: {
    color: palette.ink4,
    fontSize: font.micro,
    fontWeight: weight.medium,
  },
  tabTextActive: {
    color: palette.brand,
    fontWeight: weight.bold,
  },
});
