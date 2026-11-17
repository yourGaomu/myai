import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ChangeEntry } from "../../../protocol";
import type { ToolCallStep } from "../../../utils/chatRenderItems";
import { baseName, dirName, parseToolArgs } from "../../../utils/toolDisplay";
import { fontFamily, font, radius, weight } from "../../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../../theme/ThemeProvider";

export type FileChangeTag = {
  action: "NEW" | "MOD" | "DEL";
  addedLines?: number;
  path: string;
  removedLines?: number;
};

export function extractFileChanges(tools: ToolCallStep[] | undefined, workspaceChanges?: ChangeEntry[]): FileChangeTag[] {
  if (!tools) return [];
  const files: FileChangeTag[] = [];
  const seen = new Set<string>();

  const findChangeEntry = (rawPath: string) => {
    if (!workspaceChanges?.length) return undefined;
    const normalized = rawPath.replace(/\\/g, "/").replace(/^\.\//, "");
    return workspaceChanges.find(
      (entry) =>
        entry.path === normalized ||
        normalized.endsWith(`/${entry.path}`) ||
        entry.path.endsWith(`/${normalized}`),
    );
  };

  for (const tool of tools) {
    const lower = tool.name.toLowerCase();
    if (!/(write|edit|patch|replace)/.test(lower) || tool.status === "error") continue;
    const args = parseToolArgs(tool.arguments);
    if (!args || typeof args !== "object") continue;
    const target = String(args.TargetFile || args.file_path || args.path || "").trim();
    if (target && !seen.has(target)) {
      seen.add(target);
      const matched = findChangeEntry(target);
      const action: "NEW" | "MOD" | "DEL" = matched?.deleted
        ? "DEL"
        : matched?.status === "added" || lower.includes("write")
          ? "NEW"
          : "MOD";
      files.push({
        action,
        addedLines: matched?.added_lines,
        path: matched?.path || target,
        removedLines: matched?.removed_lines,
      });
    }
  }
  return files;
}

type Props = {
  files: FileChangeTag[];
  onOpenChangePath?: (path: string) => void;
  onOpenChanges?: () => void;
};

// 回答下方的变更摘要：展示 +X/-Y 行变更统计，点击单行直达该文件 Diff，底部进入变更总表。
export function TurnChanges({ files, onOpenChangePath, onOpenChanges }: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);

  if (files.length === 0) {
    return null;
  }
  return (
    <View style={styles.card}>
      <Pressable
        accessibilityRole="button"
        disabled={!onOpenChanges}
        onPress={onOpenChanges}
        style={({ pressed }) => [styles.head, pressed && styles.pressed]}
      >
        <Feather color={palette.ink1} name="git-pull-request" size={16} />
        <Text style={styles.title}>修改了 {files.length} 个文件</Text>
      </Pressable>
      {files.map((file) => {
        const dir = dirName(file.path);
        const canTap = Boolean(onOpenChangePath || onOpenChanges);
        const letter = file.action === "NEW" ? "A" : file.action === "DEL" ? "D" : "M";
        const letterStyle =
          file.action === "NEW" ? styles.letterNew : file.action === "DEL" ? styles.letterDel : styles.letterMod;

        return (
          <Pressable
            accessibilityRole="button"
            disabled={!canTap}
            key={file.path}
            onPress={() => {
              if (onOpenChangePath) {
                onOpenChangePath(file.path);
              } else {
                onOpenChanges?.();
              }
            }}
            style={({ pressed }) => [styles.row, pressed && styles.pressed]}
          >
            <Text style={[styles.letter, letterStyle]}>{letter}</Text>
            <Text numberOfLines={1} style={styles.name}>
              {baseName(file.path)}
              {dir ? <Text style={styles.dir}>  {dir}</Text> : null}
            </Text>
            {(file.addedLines || 0) > 0 || (file.removedLines || 0) > 0 ? (
              <View style={styles.stats}>
                {(file.addedLines || 0) > 0 ? <Text style={styles.statAdd}>+{file.addedLines}</Text> : null}
                {(file.removedLines || 0) > 0 ? <Text style={styles.statDel}>−{file.removedLines}</Text> : null}
              </View>
            ) : null}
            {onOpenChangePath ? <Feather color={palette.ink4} name="chevron-right" size={13} /> : null}
          </Pressable>
        );
      })}
      {onOpenChanges ? (
        <Pressable
          accessibilityRole="button"
          onPress={onOpenChanges}
          style={({ pressed }) => [styles.footer, pressed && styles.pressed]}
        >
          <Text style={styles.footerText}>查看全部变更</Text>
          <Feather color={palette.brandLight} name="chevron-right" size={15} />
        </Pressable>
      ) : null}
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    card: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.lg,
      borderWidth: 1,
      overflow: "hidden",
    },
    pressed: { backgroundColor: palette.pressed },
    head: { alignItems: "center", flexDirection: "row", gap: 8, paddingBottom: 6, paddingHorizontal: 14, paddingTop: 12 },
    title: { color: palette.ink1, flex: 1, fontSize: font.label + 0.5, fontWeight: weight.semibold },
    row: { alignItems: "center", flexDirection: "row", gap: 8, minHeight: 42, paddingHorizontal: 14 },
    letter: { fontFamily: fontFamily.mono, fontSize: font.caption, fontWeight: weight.semibold, textAlign: "center", width: 16 },
    letterNew: { color: palette.ok },
    letterMod: { color: palette.warn },
    letterDel: { color: palette.err },
    name: { color: palette.ink1, flex: 1, fontSize: font.label + 0.5 },
    dir: { color: palette.ink3, fontSize: font.caption - 0.5 },
    stats: { alignItems: "center", flexDirection: "row", gap: 5 },
    statAdd: { color: palette.diffAdd, fontFamily: fontFamily.mono, fontSize: 11.5, fontWeight: weight.semibold },
    statDel: { color: palette.diffDel, fontFamily: fontFamily.mono, fontSize: 11.5, fontWeight: weight.semibold },
    footer: {
      alignItems: "center",
      borderTopColor: palette.hairline,
      borderTopWidth: 1,
      flexDirection: "row",
      gap: 4,
      justifyContent: "center",
      minHeight: 42,
    },
    footerText: { color: palette.brandLight, fontSize: font.label },
  });
