import { Platform } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { androidBottomInsetFallback } from "../constants/app";

// 根据安全区、窗口高度和 Token 栏状态计算稳定布局尺寸，避免键盘与底栏遮挡内容。
export function useMobileLayoutMetrics() {
  const insets = useSafeAreaInsets();
  const bottomSafePadding = Math.max(insets.bottom, Platform.OS === "android" ? androidBottomInsetFallback : 12);
  const topSafePadding = Math.max(insets.top, 10) + 6;

  return {
    bottomSafePadding,
    topSafePadding,
  };
}
