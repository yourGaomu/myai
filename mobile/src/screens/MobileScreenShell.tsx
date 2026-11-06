import { useEffect, type ReactNode } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

import { palette, space } from "../constants/theme";

type Props = {
  bottomDock: ReactNode;
  bottomSafePadding: number;
  children: ReactNode;
  // 顶栏固定在滚动区外面，自己处理状态栏安全区
  header: ReactNode;
  scrollEnabled?: boolean;
};

export function MobileScreenShell({ bottomDock, bottomSafePadding, children, header, scrollEnabled = true }: Props) {
  useEffect(() => {
    if (Platform.OS === "web" && typeof document !== "undefined") {
      document.body.style.backgroundColor = palette.canvas;
    }
  }, []);

  return (
    <SafeAreaView edges={["left", "right"]} style={styles.safe}>
      <StatusBar style="light" />
      <KeyboardAvoidingView
        behavior={Platform.OS === "web" ? undefined : "padding"}
        keyboardVerticalOffset={0}
        style={styles.keyboard}
      >
        {header}
        {scrollEnabled ? (
          <ScrollView
            contentContainerStyle={[styles.content, { paddingBottom: bottomSafePadding + 20 }]}
            keyboardShouldPersistTaps="handled"
            showsVerticalScrollIndicator={false}
            style={styles.scrollArea}
          >
            {children}
          </ScrollView>
        ) : (
          <View style={[styles.content, styles.fixedContent]}>{children}</View>
        )}
        {bottomDock}
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: {
    backgroundColor: palette.canvas,
    flex: 1,
  },
  keyboard: {
    flex: 1,
  },
  scrollArea: {
    flex: 1,
  },
  content: {
    gap: space.lg,
    paddingHorizontal: space.xl,
    paddingTop: 4,
  },
  fixedContent: {
    flex: 1,
    minHeight: 0,
  },
});
