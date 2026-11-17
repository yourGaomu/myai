import { useEffect, type ReactNode } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";

import { space } from "../constants/theme";
import { useAppTheme, useThemedStyles } from "../theme/ThemeProvider";

type Props = {
  backgroundLayer?: ReactNode;
  bottomDock: ReactNode;
  bottomSafePadding: number;
  children: ReactNode;
  floatingHeader?: boolean;
  // floatingHeader=true 时顶栏以透明悬浮层覆盖在主内容上方，让对话消息滑过顶栏背部
  header: ReactNode;
  scrollEnabled?: boolean;
};

export function MobileScreenShell({
  backgroundLayer,
  bottomDock,
  bottomSafePadding,
  children,
  floatingHeader = false,
  header,
  scrollEnabled = true,
}: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);

  useEffect(() => {
    if (Platform.OS === "web" && typeof document !== "undefined") {
      document.body.style.backgroundColor = palette.canvas;
    }
  }, [palette.canvas]);

  return (
    <SafeAreaView edges={["left", "right"]} style={styles.safe}>
      {backgroundLayer}
      <KeyboardAvoidingView
        behavior={Platform.OS === "web" ? undefined : "padding"}
        keyboardVerticalOffset={0}
        style={styles.keyboard}
      >
        {floatingHeader ? null : header}
        <View style={styles.stage}>
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
            <View style={[styles.content, styles.fixedContent, floatingHeader && styles.fullBleedContent]}>{children}</View>
          )}
          {floatingHeader ? (
            <View pointerEvents="box-none" style={styles.floatingOverlay}>
              {header}
            </View>
          ) : null}
        </View>
        {bottomDock}
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    safe: {
      backgroundColor: palette.canvas,
      flex: 1,
    },
    keyboard: {
      flex: 1,
    },
    stage: {
      flex: 1,
      minHeight: 0,
      position: "relative",
    },
    floatingOverlay: {
      left: 0,
      position: "absolute",
      right: 0,
      top: 0,
      zIndex: 25,
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
    fullBleedContent: {
      paddingHorizontal: 0,
      paddingTop: 0,
    },
  });
