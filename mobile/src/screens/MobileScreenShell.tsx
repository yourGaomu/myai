import type { ReactNode } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

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
  return (
    <SafeAreaView edges={["left", "right"]} style={styles.safe}>
      <StatusBar style="dark" />
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : "height"}
        keyboardVerticalOffset={0}
        style={styles.keyboard}
      >
        <View style={styles.screen}>
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
                  paddingBottom: 8,
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
    backgroundColor: "#f4f5f7",
  },
  keyboard: {
    flex: 1,
  },
  screen: {
    flex: 1,
  },
  scrollArea: {
    flex: 1,
  },
  content: {
    gap: 12,
    paddingHorizontal: 14,
  },
  fixedContent: {
    flex: 1,
    minHeight: 0,
  },
});
