import { useEffect, type ReactNode } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

import { palette, space } from "../constants/theme";

type Props = {
  bottomDock: ReactNode;
  bottomSafePadding: number;
  children: ReactNode;
  scrollEnabled?: boolean;
  topSafePadding: number;
};

export function MobileScreenShell({
  bottomDock,
  bottomSafePadding,
  children,
  scrollEnabled = true,
  topSafePadding,
}: Props) {
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
        <View style={styles.screen}>
          {/* 顶部品牌环境光：纯装饰，不参与布局与交互 */}
          <View pointerEvents="none" style={styles.brandGlow} />
          {scrollEnabled ? (
            <ScrollView
              contentContainerStyle={[
                styles.content,
                {
                  paddingTop: topSafePadding,
                  paddingBottom: bottomSafePadding + 20,
                },
              ]}
              keyboardShouldPersistTaps="handled"
              showsVerticalScrollIndicator={false}
              style={styles.scrollArea}
            >
              {children}
            </ScrollView>
          ) : (
            <View
              style={[
                styles.content,
                styles.fixedContent,
                {
                  paddingTop: topSafePadding,
                  paddingBottom: 2,
                },
              ]}
            >
              {children}
            </View>
          )}
          {bottomDock}
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: {
    flex: 1,
    backgroundColor: palette.canvas,
  },
  keyboard: {
    flex: 1,
  },
  screen: {
    flex: 1,
  },
  // RN 无 radial-gradient，用一个大圆 + 品牌色柔光模拟顶部环境光
  brandGlow: {
    position: "absolute",
    top: -140,
    alignSelf: "center",
    width: 360,
    height: 300,
    borderRadius: 180, // = width/2，用于画正圆，非设计 token
    backgroundColor: palette.ambientGlow,
    shadowColor: palette.brand,
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.35,
    shadowRadius: 90,
    opacity: 0.5,
    zIndex: 0,
  },
  scrollArea: {
    flex: 1,
  },
  content: {
    gap: space.lg,
    paddingHorizontal: space.xl,
  },
  fixedContent: {
    flex: 1,
    minHeight: 0,
  },
});
