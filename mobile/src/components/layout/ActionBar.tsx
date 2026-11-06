import type { ComponentProps } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, palette, radius, weight } from "../../constants/theme";

export type BarAction = {
  disabled?: boolean;
  icon: ComponentProps<typeof Feather>["name"];
  label: string;
  loading?: boolean;
  onPress: () => void;
  tone: "primary" | "secondary" | "danger";
};

type Props = {
  actions: BarAction[];
  bottomPadding: number;
};

// 二级页面底部的固定操作栏（文件预览的「添加到对话」、Diff 的「打开文件 / 还原」）。
export function ActionBar({ actions, bottomPadding }: Props) {
  return (
    <View style={[styles.bar, { paddingBottom: bottomPadding }]}>
      {actions.map((action) => {
        const color = action.tone === "primary" ? palette.inkInverse : action.tone === "danger" ? palette.err : palette.ink1;
        return (
          <Pressable
            accessibilityRole="button"
            accessibilityState={{ disabled: action.disabled }}
            disabled={action.disabled || action.loading}
            key={action.label}
            onPress={action.onPress}
            style={({ pressed }) => [styles.button, styles[action.tone], action.disabled && styles.disabled, pressed && styles.pressed]}
          >
            {action.loading ? <ActivityIndicator color={color} size="small" /> : <Feather color={color} name={action.icon} size={17} />}
            <Text style={[styles.label, { color }]}>{action.label}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    backgroundColor: palette.canvas,
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    flexDirection: "row",
    gap: 10,
    paddingHorizontal: 16,
    paddingTop: 10,
  },
  button: {
    alignItems: "center",
    borderRadius: radius.md,
    flex: 1,
    flexDirection: "row",
    gap: 6,
    justifyContent: "center",
    minHeight: 46,
  },
  primary: { backgroundColor: palette.brand },
  secondary: { backgroundColor: palette.well },
  danger: { backgroundColor: palette.errWash },
  disabled: { opacity: 0.4 },
  pressed: { opacity: 0.8 },
  label: { fontSize: font.body - 0.5, fontWeight: weight.semibold },
});
