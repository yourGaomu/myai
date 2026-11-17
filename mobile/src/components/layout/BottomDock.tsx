import type { ComponentProps } from "react";
import { StyleSheet, View } from "react-native";

import { space } from "../../constants/theme";
import { useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import type { ViewMode } from "../../types/app";
import { Composer } from "../chat/Composer";

type Props = ComponentProps<typeof Composer> & {
  bottomPadding: number;
  viewMode: ViewMode;
};

// 只有对话页有底栏（输入区）；其他页面通过侧栏和顶栏返回。
export function BottomDock({ bottomPadding, viewMode, ...composer }: Props) {
  const styles = useThemedStyles(makeStyles);
  if (viewMode !== "chat") {
    return null;
  }
  return (
    <View style={[styles.dock, { paddingBottom: bottomPadding }]}>
      <Composer {...composer} />
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    dock: {
      backgroundColor: palette.canvas,
      paddingHorizontal: space.lg,
      paddingTop: space.sm,
    },
  });
