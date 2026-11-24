import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  Animated,
  Easing,
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
import { Feather } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type { ButtonFeedback } from "../../types/ui";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { useReduceMotion } from "./MotionEffects";

type Props = {
  buttonFeedback: ButtonFeedback;
  children: ReactNode;
  footer: ReactNode;
  onClose: () => void;
  title: string;
  visible: boolean;
};

// 全平台响应式表单弹窗：遮罩独立淡入淡出，内容紧凑屏底部滑入/宽屏缩放弹出，退场期间保留快照防闪烁。
export function ResponsiveFormModal({
  buttonFeedback,
  children,
  footer,
  onClose,
  title,
  visible,
}: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const { height, width } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const compact = width < 700;
  const availableHeight = Math.max(360, height - Math.max(insets.top, 12) - (compact ? 0 : 48));

  const snapshotRef = useRef({ children, footer, title });
  if (visible) {
    snapshotRef.current = { children, footer, title };
  }
  const shown = visible ? { children, footer, title } : snapshotRef.current;

  const [mounted, setMounted] = useState(visible);
  const mountedRef = useRef(visible);
  const fade = useRef(new Animated.Value(visible ? 1 : 0)).current;
  const sheetAnim = useRef(new Animated.Value(visible ? 0 : 1)).current;

  useEffect(() => {
    fade.stopAnimation();
    sheetAnim.stopAnimation();
    if (visible) {
      if (!mountedRef.current) {
        mountedRef.current = true;
        setMounted(true);
        fade.setValue(reduceMotion ? 1 : 0);
        sheetAnim.setValue(reduceMotion ? 0 : 1);
      } else if (reduceMotion) {
        fade.setValue(1);
        sheetAnim.setValue(0);
      }
      if (reduceMotion) {
        return;
      }
      Animated.parallel([
        Animated.timing(fade, { duration: 180, easing: Easing.out(Easing.quad), toValue: 1, useNativeDriver: true }),
        Animated.spring(sheetAnim, { bounciness: 0, speed: 18, toValue: 0, useNativeDriver: true }),
      ]).start();
      return;
    }
    if (!mountedRef.current) {
      return;
    }
    if (reduceMotion) {
      mountedRef.current = false;
      setMounted(false);
      return;
    }
    Animated.parallel([
      Animated.timing(fade, { duration: 150, easing: Easing.in(Easing.quad), toValue: 0, useNativeDriver: true }),
      Animated.timing(sheetAnim, { duration: 170, easing: Easing.in(Easing.quad), toValue: 1, useNativeDriver: true }),
    ]).start(({ finished }) => {
      if (finished) {
        mountedRef.current = false;
        setMounted(false);
      }
    });
  }, [fade, reduceMotion, sheetAnim, visible]);

  if (!mounted) {
    return null;
  }

  const sheetMotionStyle = compact
    ? { transform: [{ translateY: sheetAnim.interpolate({ inputRange: [0, 1], outputRange: [0, 420] }) }] }
    : { opacity: fade, transform: [{ scale: sheetAnim.interpolate({ inputRange: [0, 1], outputRange: [1, 0.94] }) }] };

  return (
    <Modal
      animationType="none"
      navigationBarTranslucent
      onRequestClose={visible ? onClose : undefined}
      presentationStyle="overFullScreen"
      statusBarTranslucent
      transparent
      visible
    >
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : Platform.OS === "android" ? "height" : undefined}
        style={styles.keyboard}
      >
        <View style={[styles.backdrop, compact ? styles.backdropCompact : styles.backdropWide]}>
          <Animated.View pointerEvents={visible ? "auto" : "none"} style={[styles.backdropScrim, { opacity: fade }]}>
            <Pressable
              accessibilityLabel="关闭编辑窗口"
              accessibilityRole="button"
              disabled={!visible}
              onPress={onClose}
              style={StyleSheet.absoluteFill}
            />
          </Animated.View>
          <Animated.View
            accessibilityViewIsModal
            style={[
              styles.sheet,
              compact ? styles.sheetCompact : styles.sheetWide,
              { maxHeight: availableHeight, paddingBottom: Math.max(insets.bottom, 12) },
              sheetMotionStyle,
            ]}
          >
            {compact ? <View style={styles.grabber} /> : null}
            <View style={styles.header}>
              <Text numberOfLines={1} style={styles.title}>{shown.title}</Text>
              <Pressable
                accessibilityLabel="关闭"
                accessibilityRole="button"
                disabled={!visible}
                hitSlop={10}
                onPress={onClose}
                style={({ pressed }) => buttonFeedback(styles.closeButton, pressed)}
              >
                <Feather color={palette.ink2} name="x" size={18} />
              </Pressable>
            </View>
            <ScrollView
              contentContainerStyle={styles.body}
              keyboardShouldPersistTaps="handled"
              showsVerticalScrollIndicator
              style={styles.scroll}
            >
              {shown.children}
            </ScrollView>
            <View style={styles.footer}>{shown.footer}</View>
          </Animated.View>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    keyboard: { flex: 1 },
    backdrop: { flex: 1, paddingHorizontal: 14, position: "relative" },
    backdropScrim: { ...StyleSheet.absoluteFill, backgroundColor: palette.scrim },
    backdropCompact: { justifyContent: "flex-end", paddingHorizontal: 0 },
    backdropWide: { alignItems: "center", justifyContent: "center", paddingVertical: 24 },
    sheet: { backgroundColor: palette.surface2, overflow: "hidden", width: "100%" },
    sheetCompact: {
      borderTopLeftRadius: radius.sheet,
      borderTopRightRadius: radius.sheet,
    },
    sheetWide: {
      borderRadius: radius.xl,
      maxWidth: 760,
    },
    header: {
      alignItems: "center",
      flexDirection: "row",
      justifyContent: "space-between",
      minHeight: 52,
      paddingLeft: 16,
      paddingRight: 10,
      paddingTop: 4,
    },
    title: { color: palette.ink1, flex: 1, fontSize: font.title, fontWeight: weight.semibold },
    closeButton: {
      alignItems: "center",
      backgroundColor: palette.well,
      borderRadius: radius.pill,
      height: 36,
      justifyContent: "center",
      width: 36,
    },
    closeText: { color: palette.ink1, fontSize: font.display, fontWeight: weight.regular, lineHeight: 30 },
    scroll: { flexShrink: 1 },
    body: {
      gap: 12,
      paddingBottom: 16,
      paddingHorizontal: 16,
      paddingTop: 4,
    },
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
    grabber: {
      alignSelf: "center",
      backgroundColor: palette.hairlineStrong,
      borderRadius: radius.xxs,
      height: 4,
      marginTop: 8,
      width: 36,
    },
  });
