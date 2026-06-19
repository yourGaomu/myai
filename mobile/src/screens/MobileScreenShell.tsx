import type { ReactNode, RefObject } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

type Props = {
  bottomDock: ReactNode;
  bottomSafePadding: number;
  children: ReactNode;
  scrollRef?: RefObject<ScrollView | null>;
  topSafePadding: number;
};

export function MobileScreenShell({
  bottomDock,
  bottomSafePadding,
  children,
  scrollRef,
  topSafePadding,
}: Props) {
  return (
    <SafeAreaView edges={["left", "right"]} style={styles.safe}>
      <StatusBar style="dark" />
      <View pointerEvents="none" style={styles.backgroundArt}>
        <View style={styles.artStripeCyan} />
        <View style={styles.artStripeYellow} />
        <View style={styles.artStripeGreen} />
      </View>
      <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.keyboard}>
        <View style={styles.screen}>
          <ScrollView
            contentContainerStyle={[
              styles.content,
              {
                paddingTop: topSafePadding,
                paddingBottom: bottomSafePadding + 20,
              },
            ]}
            keyboardShouldPersistTaps="handled"
            ref={scrollRef}
            showsVerticalScrollIndicator={false}
            style={styles.scrollArea}
          >
            {children}
          </ScrollView>
          {bottomDock}
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: {
    flex: 1,
    backgroundColor: "#efe4d2",
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
  backgroundArt: {
    bottom: 0,
    left: 0,
    overflow: "hidden",
    position: "absolute",
    right: 0,
    top: 0,
  },
  artStripeCyan: {
    backgroundColor: "#4fd7ee",
    height: 220,
    left: -90,
    opacity: 0.28,
    position: "absolute",
    top: 120,
    transform: [{ rotate: "-16deg" }],
    width: 240,
  },
  artStripeYellow: {
    backgroundColor: "#ffd84f",
    height: 180,
    opacity: 0.24,
    position: "absolute",
    right: -70,
    top: 240,
    transform: [{ rotate: "12deg" }],
    width: 210,
  },
  artStripeGreen: {
    backgroundColor: "#b9e9b0",
    bottom: 220,
    height: 200,
    left: 30,
    opacity: 0.2,
    position: "absolute",
    transform: [{ rotate: "8deg" }],
    width: 160,
  },
  content: {
    gap: 12,
    paddingHorizontal: 14,
  },
});
