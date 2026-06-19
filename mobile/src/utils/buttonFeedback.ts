import type { StyleProp, ViewStyle } from "react-native";

export function buttonFeedback(style: StyleProp<ViewStyle>, active?: boolean): StyleProp<ViewStyle> {
  return [style, active ? { opacity: 0.72, transform: [{ translateX: 2 }, { translateY: 2 }] } : null];
}
