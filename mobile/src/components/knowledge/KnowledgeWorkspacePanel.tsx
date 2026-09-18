import { useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import type { ButtonFeedback } from "../../types/ui";
import { AIMemoryPanel, type AIMemoryPanelProps } from "./AIMemoryPanel";
import { KnowledgePanel, type KnowledgePanelProps } from "./KnowledgePanel";

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
            <Text style={[styles.tabText, mode === "documents" && styles.tabTextActive]}>📚 资料知识库</Text>
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
            <Text style={[styles.tabText, mode === "memory" && styles.tabTextActive]}>🧠 AI 记忆</Text>
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
    backgroundColor: "#f4f5f7",
    paddingHorizontal: 14,
    paddingTop: 10,
    paddingBottom: 4,
  },
  tabs: {
    backgroundColor: "#e6dfd3",
    borderColor: "#25231f",
    borderRadius: 999,
    borderWidth: 2,
    flexDirection: "row",
    gap: 3,
    padding: 3,
  },
  tab: {
    alignItems: "center",
    borderRadius: 999,
    flex: 1,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  tabActive: {
    backgroundColor: "#fffdf7",
    borderColor: "#25231f",
    borderWidth: 1.5,
    elevation: 2,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.15,
    shadowRadius: 1,
  },
  tabText: {
    color: "#6c665f",
    fontSize: 13,
    fontWeight: "800",
  },
  tabTextActive: {
    color: "#12100e",
    fontWeight: "900",
  },
});
