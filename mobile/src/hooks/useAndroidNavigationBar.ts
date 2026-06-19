import { useEffect } from "react";
import * as NavigationBar from "expo-navigation-bar";
import { Platform } from "react-native";

export function useAndroidNavigationBar() {
  useEffect(() => {
    if (Platform.OS !== "android") {
      return;
    }

    NavigationBar.NavigationBar.setStyle("dark");
    NavigationBar.NavigationBar.setHidden(true);
    NavigationBar.setVisibilityAsync("hidden").catch(() => undefined);
  }, []);
}
