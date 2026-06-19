import type { StyleProp, ViewStyle } from "react-native";

export type ButtonFeedback = (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
