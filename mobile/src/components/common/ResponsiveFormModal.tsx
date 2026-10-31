import type { ReactNode } from "react";
import {
  KeyboardAvoidingView,
  Modal,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  useWindowDimensions,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, weight } from "../../constants/theme";

type Props = {
  buttonFeedback: ButtonFeedback;
  children: ReactNode;
  footer: ReactNode;
  onClose: () => void;
  title: string;
  visible: boolean;
};

export function ResponsiveFormModal({
  buttonFeedback,
  children,
  footer,
  onClose,
  title,
  visible,
}: Props) {
  const { height, width } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const compact = width < 700;
  const availableHeight = Math.max(360, height - Math.max(insets.top, 12) - (compact ? 0 : 48));

  return (
    <Modal
      animationType="slide"
      onRequestClose={onClose}
      presentationStyle="overFullScreen"
      statusBarTranslucent
      transparent
      visible={visible}
    >
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : Platform.OS === "android" ? "height" : undefined}
        style={styles.keyboard}
      >
        <View style={[styles.backdrop, compact ? styles.backdropCompact : styles.backdropWide]}>
          <Pressable
            accessibilityLabel="关闭编辑窗口"
            accessibilityRole="button"
            onPress={onClose}
            style={StyleSheet.absoluteFill}
          />
          <View
            accessibilityViewIsModal
            style={[
              styles.sheet,
              compact ? styles.sheetCompact : styles.sheetWide,
              { maxHeight: availableHeight, paddingBottom: Math.max(insets.bottom, 12) },
            ]}
          >
            <View style={styles.header}>
              <Text numberOfLines={1} style={styles.title}>{title}</Text>
              <Pressable
                accessibilityLabel="关闭"
                accessibilityRole="button"
                hitSlop={10}
                onPress={onClose}
                style={({ pressed }) => buttonFeedback(styles.closeButton, pressed)}
              >
                <Text style={styles.closeText}>×</Text>
              </Pressable>
            </View>
            <ScrollView
              contentContainerStyle={styles.body}
              keyboardShouldPersistTaps="handled"
              showsVerticalScrollIndicator
              style={styles.scroll}
            >
              {children}
            </ScrollView>
            <View style={styles.footer}>{footer}</View>
          </View>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  keyboard: { flex: 1 },
  backdrop: { backgroundColor: palette.scrim, flex: 1, paddingHorizontal: 14 },
  backdropCompact: { justifyContent: "flex-end", paddingHorizontal: 0 },
  backdropWide: { alignItems: "center", justifyContent: "center", paddingVertical: 24 },
  sheet: { backgroundColor: palette.surface2, overflow: "hidden", width: "100%" },
  sheetCompact: { borderTopLeftRadius: radius.sm, borderTopRightRadius: radius.sm },
  sheetWide: { borderRadius: radius.sm, maxWidth: 760 },
  header: {
    alignItems: "center",
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    minHeight: 56,
    paddingHorizontal: 16,
  },
  title: { color: palette.ink1, flex: 1, fontSize: font.title, fontWeight: weight.bold },
  closeButton: { alignItems: "center", borderRadius: radius.sm, height: 38, justifyContent: "center", width: 38 },
  closeText: { color: palette.ink1, fontSize: font.display, fontWeight: weight.regular, lineHeight: 30 },
  scroll: { flexShrink: 1 },
  body: { gap: 12, padding: 16 },
  footer: {
    backgroundColor: palette.surface2,
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    flexDirection: "row",
    gap: 10,
    justifyContent: "flex-end",
    paddingHorizontal: 16,
    paddingTop: 12,
  },
});
