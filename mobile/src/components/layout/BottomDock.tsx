import type { ComponentProps } from "react";
import { StyleSheet, View } from "react-native";

import { palette, space } from "../../constants/theme";
import type { ViewMode } from "../../types/app";
import { Composer } from "../chat/Composer";

type Props = ComponentProps<typeof Composer> & {
  bottomPadding: number;
  viewMode: ViewMode;
};

// 只有对话页有底栏（输入区）；其他页面通过侧栏和顶栏返回，不再显示底部标签栏。
export function BottomDock({ bottomPadding, viewMode, ...composer }: Props) {
  if (viewMode !== "chat") {
    return null;
  }
  return (
    <View style={[styles.dock, { paddingBottom: bottomPadding }]}>
      <Composer {...composer} />
    </View>
  );
}

const styles = StyleSheet.create({
  dock: {
    backgroundColor: palette.canvas,
    paddingHorizontal: space.lg,
    paddingTop: space.sm,
  },
});
