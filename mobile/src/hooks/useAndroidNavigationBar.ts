import { useEffect } from "react";
import * as NavigationBar from "expo-navigation-bar";
import { Platform } from "react-native";

export function useAndroidNavigationBar(isDark = true) {
  useEffect(() => {
    if (Platform.OS !== "android") {
      return;
    }

    try {
      // setStyle 控制导航栏图标明暗：深色背景配浅色图标("light")，浅色背景配深色图标("dark")。
      NavigationBar.setStyle?.(isDark ? "light" : "dark");
      NavigationBar.setVisibilityAsync?.("visible").catch(() => undefined);
    } catch {
      // ignore
    }
  }, [isDark]);
}
