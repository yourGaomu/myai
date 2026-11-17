import { ActivityIndicator, Modal, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";

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

// 全平台一致的二次确认模态框：替代 iOS/Android 外观割裂的原生 Alert。
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
  const { palette } = theme;
  const accentColor = danger ? palette.err : palette.brand;

  return (
    <Modal animationType="fade" onRequestClose={pending ? undefined : onCancel} statusBarTranslucent transparent visible={visible}>
      <View style={styles.backdrop}>
        <Pressable disabled={pending} onPress={onCancel} style={StyleSheet.absoluteFill} />
        <View style={styles.card}>
          <View style={styles.head}>
            <View style={[styles.iconBox, { backgroundColor: danger ? palette.errWash : palette.brandWash, borderColor: danger ? palette.errBorder : palette.brandDeep }]}>
              <Feather color={accentColor} name={icon} size={18} />
            </View>
            <Text style={styles.title}>{title}</Text>
          </View>
          <Text style={styles.message}>{message}</Text>
          <View style={styles.actions}>
            <Pressable
              accessibilityRole="button"
              disabled={pending}
              onPress={onCancel}
              style={({ pressed }) => [styles.btn, styles.cancelBtn, pressed && styles.pressed]}
            >
              <Text style={styles.cancelText}>{cancelLabel}</Text>
            </Pressable>
            <Pressable
              accessibilityRole="button"
              disabled={pending}
              onPress={onConfirm}
              style={({ pressed }) => [
                styles.btn,
                danger ? styles.dangerBtn : styles.primaryBtn,
                pressed && styles.pressed,
                pending && styles.disabled,
              ]}
            >
              {pending ? (
                <ActivityIndicator color={danger ? "#FFFFFF" : palette.inkInverse} size="small" />
              ) : (
                <Text style={[styles.confirmText, { color: danger ? "#FFFFFF" : palette.inkInverse }]}>{confirmLabel}</Text>
              )}
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const makeStyles = ({ font, palette, shadow }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    backdrop: {
      alignItems: "center",
      backgroundColor: palette.scrim,
      flex: 1,
      justifyContent: "center",
      padding: 24,
    },
    card: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.xl,
      borderWidth: 1,
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
    message: {
      color: palette.ink2,
      fontSize: font.body,
      lineHeight: 22,
      marginBottom: 18,
    },
    actions: {
      flexDirection: "row",
      gap: 10,
      justifyContent: "flex-end",
    },
    btn: {
      alignItems: "center",
      borderRadius: radius.md,
      justifyContent: "center",
      minHeight: 42,
      minWidth: 96,
      paddingHorizontal: 16,
    },
    cancelBtn: {
      backgroundColor: palette.surface3,
      borderColor: palette.hairline,
      borderWidth: 1,
    },
    primaryBtn: {
      backgroundColor: palette.brand,
    },
    dangerBtn: {
      backgroundColor: "#E05242",
    },
    cancelText: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    confirmText: {
      fontSize: font.body,
      fontWeight: weight.bold,
    },
    pressed: {
      opacity: 0.82,
    },
    disabled: {
      opacity: 0.55,
    },
  });
