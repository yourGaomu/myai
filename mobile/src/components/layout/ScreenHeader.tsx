import type { ComponentProps, ReactNode } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, palette, radius, weight } from "../../constants/theme";

type FeatherName = ComponentProps<typeof Feather>["name"];

type IconButtonProps = {
  accessibilityLabel: string;
  disabled?: boolean;
  icon: FeatherName;
  muted?: boolean;
  onPress?: () => void;
  size?: number;
};

// 顶栏图标按钮：44×44 触控区，无边框，按下时出现圆形底色。
export function HeaderIconButton({ accessibilityLabel, disabled, icon, muted, onPress, size = 21 }: IconButtonProps) {
  return (
    <Pressable
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      disabled={disabled}
      hitSlop={4}
      onPress={onPress}
      style={({ pressed }) => [styles.iconButton, pressed && styles.iconButtonPressed, disabled && styles.disabled]}
    >
      <Feather color={muted ? palette.ink2 : palette.ink1} name={icon} size={size} />
    </Pressable>
  );
}

type Props = {
  onBack?: () => void;
  right?: ReactNode;
  subtitle?: string;
  title: string;
  topInset: number;
};

// 二级页面顶栏：返回 + 标题 + 右侧操作。有副标题时标题居中，否则靠左。
export function ScreenHeader({ onBack, right, subtitle, title, topInset }: Props) {
  return (
    <View style={[styles.bar, { paddingTop: topInset }]}>
      <View style={styles.row}>
        {onBack ? (
          <HeaderIconButton accessibilityLabel="返回" icon="arrow-left" onPress={onBack} />
        ) : (
          <View style={styles.slot} />
        )}
        <View style={[styles.titleBlock, subtitle ? styles.titleCentered : null]}>
          <Text numberOfLines={1} style={styles.title}>
            {title}
          </Text>
          {subtitle ? (
            <Text numberOfLines={1} style={styles.subtitle}>
              {subtitle}
            </Text>
          ) : null}
        </View>
        <View style={styles.right}>{right ?? <View style={styles.slot} />}</View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    backgroundColor: palette.canvas,
    paddingHorizontal: 6,
  },
  row: {
    alignItems: "center",
    flexDirection: "row",
    gap: 4,
    minHeight: 52,
  },
  slot: {
    height: 44,
    width: 44,
  },
  titleBlock: {
    flex: 1,
    minWidth: 0,
    paddingLeft: 2,
  },
  titleCentered: {
    alignItems: "center",
    paddingLeft: 0,
  },
  title: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.semibold,
  },
  subtitle: {
    color: palette.ink2,
    fontSize: font.caption - 0.5,
    marginTop: 1,
  },
  right: {
    alignItems: "center",
    flexDirection: "row",
    minWidth: 44,
  },
  iconButton: {
    alignItems: "center",
    borderRadius: radius.pill,
    height: 44,
    justifyContent: "center",
    width: 44,
  },
  iconButtonPressed: {
    backgroundColor: palette.pressed,
  },
  disabled: {
    opacity: 0.4,
  },
});
