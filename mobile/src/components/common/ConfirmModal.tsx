import { useEffect, useRef, useState } from "react";
import { ActivityIndicator, Animated, Easing, Modal, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import { useReduceMotion } from "./MotionEffects";

export type ConfirmModalProps = {
  cancelLabel?: string;
  confirmLabel?: string;
  danger?: boolean;
  icon?: "alert-triangle" | "trash-2" | "rotate-ccw" | "info";
  message: string;
  onCancel: () => void;
  onConfirm: () => void;
  pending?: boolean;
  title: string;
  visible: boolean;
};

// 全平台一致的二次确认模态框：带弹簧缩放弹出与危险警示脉冲环。
export function ConfirmModal({
  cancelLabel = "取消",
  confirmLabel = "确认",
  danger = false,
  icon = "alert-triangle",
  message,
  onCancel,
  onConfirm,
  pending = false,
  title,
  visible,
}: ConfirmModalProps) {
  const theme = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const reduceMotion = useReduceMotion();
  const { palette } = theme;

  // 退场动画期间冻结最后一次展示内容，避免父层提前置空目标对象引发文案闪变。
  const snapshotRef = useRef({ cancelLabel, confirmLabel, danger, icon, message, title });
  if (visible) {
    snapshotRef.current = { cancelLabel, confirmLabel, danger, icon, message, title };
  }
  const shown = visible ? { cancelLabel, confirmLabel, danger, icon, message, title } : snapshotRef.current;
  const accentColor = shown.danger ? palette.err : palette.brand;

  const [mounted, setMounted] = useState(visible);
  const mountedRef = useRef(visible);
  const fade = useRef(new Animated.Value(visible ? 1 : 0)).current;
  const scale = useRef(new Animated.Value(visible ? 1 : 0.9)).current;
  const pulse = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    fade.stopAnimation();
    scale.stopAnimation();
    if (visible) {
      if (!mountedRef.current) {
        mountedRef.current = true;
        setMounted(true);
        if (reduceMotion) {
          fade.setValue(1);
          scale.setValue(1);
          return;
        }
        fade.setValue(0);
        scale.setValue(0.88);
      } else if (reduceMotion) {
        fade.setValue(1);
        scale.setValue(1);
        return;
      }
      Animated.parallel([
        Animated.timing(fade, { duration: 180, easing: Easing.out(Easing.quad), toValue: 1, useNativeDriver: true }),
        Animated.spring(scale, { bounciness: 6, speed: 20, toValue: 1, useNativeDriver: true }),
      ]).start();
      return;
    }
    if (!mountedRef.current) return;
    if (reduceMotion) {
      mountedRef.current = false;
      setMounted(false);
      return;
    }
    Animated.parallel([
      Animated.timing(fade, { duration: 140, easing: Easing.in(Easing.quad), toValue: 0, useNativeDriver: true }),
      Animated.timing(scale, { duration: 140, easing: Easing.in(Easing.quad), toValue: 0.92, useNativeDriver: true }),
    ]).start(({ finished }) => {
      if (finished) {
        mountedRef.current = false;
        setMounted(false);
      }
    });
  }, [fade, reduceMotion, scale, visible]);

  useEffect(() => {
    pulse.stopAnimation();
    pulse.setValue(0);
    if (!visible || !shown.danger || reduceMotion) return;
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, { duration: 1500, easing: Easing.out(Easing.cubic), toValue: 1, useNativeDriver: true }),
        Animated.delay(250),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [pulse, reduceMotion, shown.danger, visible]);

  if (!mounted) {
    return null;
  }

  const ringScale = pulse.interpolate({ inputRange: [0, 1], outputRange: [0.96, 1.38] });
  const ringOpacity = pulse.interpolate({ inputRange: [0, 1], outputRange: [0.5, 0] });
  const actionsDisabled = pending || !visible;

  return (
    <Modal animationType="none" onRequestClose={actionsDisabled ? undefined : onCancel} statusBarTranslucent transparent visible>
      <View style={styles.root}>
        <Animated.View style={[styles.backdrop, { opacity: fade }]}>
          <Pressable disabled={actionsDisabled} onPress={onCancel} style={StyleSheet.absoluteFill} />
        </Animated.View>
        <Animated.View style={[styles.card, { opacity: fade, transform: [{ scale }] }]}>
          <View style={styles.head}>
            <View style={styles.iconWrap}>
              {shown.danger && !reduceMotion ? (
                <Animated.View
                  pointerEvents="none"
                  style={[
                    styles.pulseRing,
                    {
                      borderColor: palette.err,
                      opacity: ringOpacity,
                      transform: [{ scale: ringScale }],
                    },
                  ]}
                />
              ) : null}
              <View
                style={[
                  styles.iconBox,
                  {
                    backgroundColor: shown.danger ? palette.errWash : palette.brandWash,
                    borderColor: shown.danger ? palette.errBorder : palette.brandDeep,
                  },
                ]}
              >
                <Feather color={accentColor} name={shown.icon} size={18} />
              </View>
            </View>
            <Text style={styles.title}>{shown.title}</Text>
          </View>
          <ScrollView style={styles.messageScroll}>
            <Text style={styles.message}>{shown.message}</Text>
          </ScrollView>
          <View style={styles.actions}>
            <Pressable
              accessibilityRole="button"
              disabled={actionsDisabled}
              onPress={onCancel}
              style={({ pressed }) => [styles.btn, styles.cancelBtn, pressed && styles.pressed]}
            >
              <Text style={styles.cancelText}>{shown.cancelLabel}</Text>
            </Pressable>
            <Pressable
              accessibilityRole="button"
              disabled={actionsDisabled}
              onPress={onConfirm}
              style={({ pressed }) => [
                styles.btn,
                shown.danger ? styles.dangerBtn : styles.primaryBtn,
                pressed && styles.pressed,
                pending && styles.disabled,
              ]}
            >
              {pending ? (
                <ActivityIndicator color={shown.danger ? "#FFFFFF" : palette.inkInverse} size="small" />
              ) : (
                <Text style={[styles.confirmText, { color: shown.danger ? "#FFFFFF" : palette.inkInverse }]}>{shown.confirmLabel}</Text>
              )}
            </Pressable>
          </View>
        </Animated.View>
      </View>
    </Modal>
  );
}

const makeStyles = ({ font, palette, shadow }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    root: {
      alignItems: "center",
      flex: 1,
      justifyContent: "center",
      padding: 24,
    },
    backdrop: {
      ...StyleSheet.absoluteFill,
      backgroundColor: palette.scrim,
    },
    card: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.xl,
      borderWidth: 1,
      maxHeight: "100%",
      maxWidth: 380,
      padding: 20,
      width: "100%",
      ...shadow.float,
    },
    head: {
      alignItems: "center",
      flexDirection: "row",
      gap: 12,
      marginBottom: 10,
    },
    iconWrap: {
      alignItems: "center",
      height: 36,
      justifyContent: "center",
      position: "relative",
      width: 36,
    },
    pulseRing: {
      borderRadius: radius.md + 3,
      borderWidth: 1.5,
      height: 36,
      position: "absolute",
      width: 36,
    },
    iconBox: {
      alignItems: "center",
      borderRadius: radius.md,
      borderWidth: 1,
      height: 36,
      justifyContent: "center",
      width: 36,
    },
    title: {
      color: palette.ink1,
      flex: 1,
      fontSize: font.title,
      fontWeight: weight.bold,
    },
    messageScroll: {
      flexGrow: 0,
      flexShrink: 1,
      marginBottom: 20,
    },
    message: {
      color: palette.ink2,
      fontSize: font.body - 0.5,
      lineHeight: 22,
    },
    actions: {
      flexDirection: "row",
      gap: 10,
    },
    btn: {
      alignItems: "center",
      borderRadius: radius.md,
      flex: 1,
      justifyContent: "center",
      minHeight: 44,
      paddingHorizontal: 14,
    },
    cancelBtn: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderWidth: 1,
    },
    primaryBtn: {
      backgroundColor: palette.brand,
    },
    dangerBtn: {
      backgroundColor: palette.err,
    },
    cancelText: {
      color: palette.ink1,
      fontSize: font.body - 0.5,
      fontWeight: weight.semibold,
    },
    confirmText: {
      fontSize: font.body - 0.5,
      fontWeight: weight.bold,
    },
    pressed: {
      opacity: 0.78,
      transform: [{ scale: 0.97 }],
    },
    disabled: {
      opacity: 0.5,
    },
  });
