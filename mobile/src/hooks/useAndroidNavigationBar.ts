import { useEffect } from "react";
import * as NavigationBar from "expo-navigation-bar";
import { Platform } from "react-native";

export function useAndroidNavigationBar() {
  useEffect(() => {
    if (Platform.OS !== "android") {
      return;
    }

    try {
      NavigationBar.setStyle?.("dark");
      NavigationBar.setVisibilityAsync?.("visible").catch(() => undefined);
    } catch {
      // ignore
    }
  }, []);
}
