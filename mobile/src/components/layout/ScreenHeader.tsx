import type { ComponentProps, ReactNode } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";

type FeatherName = ComponentProps<typeof Feather>["name"];

type IconButtonProps = {
  accessibilityLabel: string;
  disabled?: boolean;
  glassy?: boolean;
  icon: FeatherName;
  muted?: boolean;
  onPress?: () => void;
  size?: number;
};

// 顶栏图标按钮：42×42 触控区；glassy 模式下呈悬浮毛玻璃胶囊，适合压在滚动对话上方。
export function HeaderIconButton({
  accessibilityLabel,
  disabled,
  glassy = false,
  icon,
  muted,
  onPress,
  size = 20,
}: IconButtonProps) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);

  return (
    <Pressable
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      disabled={disabled}
      hitSlop={6}
      onPress={onPress}
      style={({ pressed }) => [
        styles.iconButton,
        glassy && styles.glassButton,
        pressed && styles.iconButtonPressed,
        disabled && styles.disabled,
      ]}
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
  const styles = useThemedStyles(makeStyles);

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

const makeStyles = ({ palette, shadow }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    bar: {
      backgroundColor: palette.canvas,
      borderBottomColor: palette.hairlineSoft,
      borderBottomWidth: 1,
      paddingHorizontal: 8,
    },
    row: {
      alignItems: "center",
      flexDirection: "row",
      gap: 4,
      minHeight: 52,
    },
    iconButton: {
      alignItems: "center",
      borderRadius: radius.pill,
      height: 42,
      justifyContent: "center",
      width: 42,
    },
    glassButton: {
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderWidth: 1,
      ...shadow.card,
    },
    iconButtonPressed: {
      backgroundColor: palette.pressed,
    },
    disabled: {
      opacity: 0.4,
    },
    slot: {
      height: 42,
      width: 42,
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
      alignItems: "flex-end",
      minWidth: 42,
    },
  });
