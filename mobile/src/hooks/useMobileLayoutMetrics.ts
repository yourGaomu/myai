import { useEffect, useState } from "react";
import { Keyboard, Platform } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { androidBottomInsetFallback } from "../constants/app";

// 根据安全区、窗口高度和键盘弹起状态计算稳定布局尺寸，避免键盘与底栏遮挡内容。
export function useMobileLayoutMetrics() {
  const insets = useSafeAreaInsets();
  const [isKeyboardVisible, setIsKeyboardVisible] = useState(false);

  useEffect(() => {
    const showEvent = Platform.OS === "ios" ? "keyboardWillShow" : "keyboardDidShow";
    const hideEvent = Platform.OS === "ios" ? "keyboardWillHide" : "keyboardDidHide";

    const showSub = Keyboard.addListener(showEvent, () => setIsKeyboardVisible(true));
    const hideSub = Keyboard.addListener(hideEvent, () => setIsKeyboardVisible(false));

    return () => {
      showSub.remove();
      hideSub.remove();
    };
  }, []);

  const defaultBottomSafe = Math.max(
    insets.bottom,
    Platform.OS === "android" ? androidBottomInsetFallback : 12,
  );
  // 软键盘弹起时，底栏已由 KeyboardAvoidingView 垫起避让系统导航区，底边距紧贴键盘（4dp）；收起后恢复手势/按键保护区
  const bottomSafePadding = isKeyboardVisible ? 4 : defaultBottomSafe;
  const topSafePadding = Math.max(insets.top, 10) + 6;

  return {
    bottomSafePadding,
    isKeyboardVisible,
    topSafePadding,
  };
}
