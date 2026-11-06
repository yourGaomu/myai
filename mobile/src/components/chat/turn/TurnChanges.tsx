import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ToolCallStep } from "../../../utils/chatRenderItems";
import { baseName, dirName, parseToolArgs } from "../../../utils/toolDisplay";
import { fontFamily, font, palette, radius, weight } from "../../../constants/theme";

export type FileChangeTag = {
  action: "NEW" | "MOD";
  path: string;
};

export function extractFileChanges(tools: ToolCallStep[] | undefined): FileChangeTag[] {
  if (!tools) return [];
  const files: FileChangeTag[] = [];
  const seen = new Set<string>();
  for (const tool of tools) {
    const lower = tool.name.toLowerCase();
    if (!/(write|edit|patch|replace)/.test(lower) || tool.status === "error") continue;
    const args = parseToolArgs(tool.arguments);
    if (!args || typeof args !== "object") continue;
    const target = String(args.TargetFile || args.file_path || args.path || "").trim();
    if (target && !seen.has(target)) {
      seen.add(target);
      files.push({ action: lower.includes("write") ? "NEW" : "MOD", path: target });
    }
  }
  return files;
}

type Props = {
  files: FileChangeTag[];
  onOpenChanges?: () => void;
};

// 回答下方的变更摘要：这一轮动过的文件，点击进入变更页查看 diff。
export function TurnChanges({ files, onOpenChanges }: Props) {
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
        <Feather color={palette.ink1} name="git-pull-request" size={17} />
        <Text style={styles.title}>修改了 {files.length} 个文件</Text>
      </Pressable>
      {files.map((file) => {
        const dir = dirName(file.path);
        return (
          <Pressable
            accessibilityRole="button"
            disabled={!onOpenChanges}
            key={file.path}
            onPress={onOpenChanges}
            style={({ pressed }) => [styles.row, pressed && styles.pressed]}
          >
            <Text style={[styles.letter, file.action === "NEW" ? styles.letterNew : styles.letterMod]}>
              {file.action === "NEW" ? "A" : "M"}
            </Text>
            <Text numberOfLines={1} style={styles.name}>
              {baseName(file.path)}
              {dir ? <Text style={styles.dir}>  {dir}</Text> : null}
            </Text>
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

const styles = StyleSheet.create({
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
  row: { alignItems: "center", flexDirection: "row", gap: 10, minHeight: 44, paddingHorizontal: 14 },
  letter: { fontFamily: fontFamily.mono, fontSize: font.caption, fontWeight: weight.semibold, textAlign: "center", width: 16 },
  letterNew: { color: palette.ok },
  letterMod: { color: palette.warn },
  name: { color: palette.ink1, flex: 1, fontSize: font.label + 0.5 },
  dir: { color: palette.ink3, fontSize: font.caption - 0.5 },
  footer: {
    alignItems: "center",
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    flexDirection: "row",
    gap: 4,
    justifyContent: "center",
    minHeight: 44,
  },
  footerText: { color: palette.brandLight, fontSize: font.label },
});
