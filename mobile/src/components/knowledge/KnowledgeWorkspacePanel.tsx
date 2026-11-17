import { useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import type { ButtonFeedback } from "../../types/ui";
import { AIMemoryPanel, type AIMemoryPanelProps } from "./AIMemoryPanel";
import { KnowledgePanel, type KnowledgePanelProps } from "./KnowledgePanel";
import { font, radius, weight } from "../../constants/theme";
import { useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";

type Props = {
  aiMemory: Omit<AIMemoryPanelProps, "buttonFeedback">;
  buttonFeedback: ButtonFeedback;
  knowledge: Omit<KnowledgePanelProps, "buttonFeedback">;
  onInsertReference?: (snippet: string) => void;
};

export function KnowledgeWorkspacePanel({ aiMemory, buttonFeedback, knowledge, onInsertReference }: Props) {
  const styles = useThemedStyles(makeStyles);
  const [mode, setMode] = useState<"documents" | "memory">("documents");
  const pendingCandidatesCount = aiMemory.candidates.filter((c) => c.status === "pending").length;

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
            <View style={styles.tabRow}>
              <Text style={[styles.tabText, mode === "memory" && styles.tabTextActive]}>AI 记忆</Text>
              {pendingCandidatesCount > 0 ? (
                <View style={styles.badge}>
                  <Text style={styles.badgeText}>{pendingCandidatesCount > 99 ? "99+" : pendingCandidatesCount}</Text>
                </View>
              ) : null}
            </View>
          </Pressable>
        </View>
      </View>
      {mode === "documents" ? (
        <KnowledgePanel buttonFeedback={buttonFeedback} onInsertReference={onInsertReference} {...knowledge} />
      ) : (
        <AIMemoryPanel buttonFeedback={buttonFeedback} onInsertReference={onInsertReference} {...aiMemory} />
      )}
    </View>
  );
}

const makeStyles = ({ palette, shadow }: AppTheme) =>
  StyleSheet.create({
    root: { flex: 1 },
    tabBarWrapper: {
      backgroundColor: palette.canvas,
      paddingBottom: 4,
      paddingTop: 6,
    },
    tabs: {
      backgroundColor: palette.well,
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
    tabRow: { alignItems: "center", flexDirection: "row", gap: 6 },
    tabText: {
      color: palette.ink2,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    tabTextActive: {
      color: palette.ink1,
      fontWeight: weight.semibold,
    },
    badge: {
      alignItems: "center",
      backgroundColor: palette.brandWashStrong,
      borderRadius: radius.pill,
      justifyContent: "center",
      minWidth: 18,
      paddingHorizontal: 5,
      paddingVertical: 1,
    },
    badgeText: {
      color: palette.brandLight,
      fontSize: 10.5,
      fontWeight: weight.bold,
    },
  });
