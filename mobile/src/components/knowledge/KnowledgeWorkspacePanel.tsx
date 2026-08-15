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
      <View style={styles.tabs}>
        <Pressable onPress={() => setMode("documents")} style={({ pressed }) => buttonFeedback([styles.tab, mode === "documents" && styles.tabActive], pressed)}>
          <Text style={[styles.tabText, mode === "documents" && styles.tabTextActive]}>资料知识库</Text>
        </Pressable>
        <Pressable onPress={() => { setMode("memory"); aiMemory.onRefreshMemories(); aiMemory.onRefreshCandidates(); }} style={({ pressed }) => buttonFeedback([styles.tab, mode === "memory" && styles.tabActive], pressed)}>
          <Text style={[styles.tabText, mode === "memory" && styles.tabTextActive]}>AI 记忆</Text>
        </Pressable>
      </View>
      {mode === "documents" ? <KnowledgePanel buttonFeedback={buttonFeedback} {...knowledge} /> : <AIMemoryPanel buttonFeedback={buttonFeedback} {...aiMemory} />}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  tabs: { backgroundColor: "#f1efe9", borderBottomColor: "#d7d2c9", borderBottomWidth: 1, flexDirection: "row", paddingHorizontal: 16, paddingTop: 8 },
  tab: { alignItems: "center", borderBottomColor: "transparent", borderBottomWidth: 2, flex: 1, minHeight: 42, justifyContent: "center" },
  tabActive: { borderBottomColor: "#1d6b52" },
  tabText: { color: "#706a62", fontSize: 14, fontWeight: "700" },
  tabTextActive: { color: "#1d211e" },
});
