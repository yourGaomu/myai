import { useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import type { ButtonFeedback } from "../../types/ui";
import { AIMemoryPanel, type AIMemoryPanelProps } from "./AIMemoryPanel";
import { KnowledgePanel, type KnowledgePanelProps } from "./KnowledgePanel";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

type Props = {
  aiMemory: Omit<AIMemoryPanelProps, "buttonFeedback">;
  buttonFeedback: ButtonFeedback;
  knowledge: Omit<KnowledgePanelProps, "buttonFeedback">;
};

export function KnowledgeWorkspacePanel({ aiMemory, buttonFeedback, knowledge }: Props) {
  const [mode, setMode] = useState<"documents" | "memory">("documents");
  return (
    <View style={styles.root}>
      <View style={styles.tabBarWrapper}>
        <View style={styles.tabs}>
          <Pressable
            onPress={() => setMode("documents")}
            style={({ pressed }) => buttonFeedback([styles.tab, mode === "documents" && styles.tabActive], pressed)}
          >
            <Text style={[styles.tabText, mode === "documents" && styles.tabTextActive]}>资料知识库</Text>
          </Pressable>
          <Pressable
            onPress={() => {
              setMode("memory");
              aiMemory.onRefreshMemories();
              aiMemory.onRefreshCandidates();
              aiMemory.onRefreshExtractionJobs();
            }}
            style={({ pressed }) => buttonFeedback([styles.tab, mode === "memory" && styles.tabActive], pressed)}
          >
            <Text style={[styles.tabText, mode === "memory" && styles.tabTextActive]}>AI 记忆</Text>
          </Pressable>
        </View>
      </View>
      {mode === "documents" ? <KnowledgePanel buttonFeedback={buttonFeedback} {...knowledge} /> : <AIMemoryPanel buttonFeedback={buttonFeedback} {...aiMemory} />}
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
  },
  tabBarWrapper: {
    backgroundColor: palette.canvas,
    paddingTop: 6,
    paddingBottom: 4,
  },
  tabs: {
    backgroundColor: palette.surface3,
    borderColor: palette.hairline,
    borderRadius: radius.pill,
    borderWidth: 1,
    flexDirection: "row",
    gap: 3,
    padding: 3,
  },
  tab: {
    alignItems: "center",
    borderRadius: radius.pill,
    flex: 1,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  tabActive: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderWidth: 1,
    ...shadow.card,
    },
  tabText: {
    color: palette.ink2,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  tabTextActive: {
    color: palette.ink1,
    fontWeight: weight.bold,
  },
});
