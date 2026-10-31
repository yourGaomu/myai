import { useEffect } from "react";
import * as NavigationBar from "expo-navigation-bar";
import { Platform } from "react-native";

export function useAndroidNavigationBar() {
  useEffect(() => {
    if (Platform.OS !== "android") {
      return;
    }

    try {
      // setStyle 指导航栏图标的明暗：深色背景需要浅色图标。
      // 背景色由 app.json 的 android.navigationBar.backgroundColor 负责，此处无对应 API。
      NavigationBar.setStyle?.("light");
      NavigationBar.setVisibilityAsync?.("visible").catch(() => undefined);
    } catch {
      // ignore
    }
  }, []);
}
