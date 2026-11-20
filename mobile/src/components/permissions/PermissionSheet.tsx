import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { PermissionState } from "../../types/app";
import { permissionBody, permissionTitle, toolCwd, toolKind } from "../../utils/toolDisplay";
import { fontFamily, font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { BottomSheet } from "../common/BottomSheet";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { PulseDot } from "../common/MotionEffects";

type Props = {
  deviceID: string;
  onAllow: () => void;
  onAllowSession?: () => void;
  onDeny: () => void;
  // 点遮罩 / 返回键：收成输入框上方的提示条，请求保持待确认
  onMinimize: () => void;
  onOpenPolicy: () => void;
  permission: PermissionState | null;
  policyLabel: string;
  visible: boolean;
};

// 工具调用权限确认：拒绝本次 / 仅允许本次 / 本会话始终允许此类权限。
export function PermissionSheet({
  deviceID,
  onAllow,
  onAllowSession,
  onDeny,
  onMinimize,
  onOpenPolicy,
  permission,
  policyLabel,
  visible,
}: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const name = permission?.name || "";
  const body = permission ? permissionBody(name, permission.arguments) : "";
  const cwd = permission ? toolCwd(permission.arguments) : "";
  const isShell = toolKind(name) === "shell";
  const scopeLabel = permission?.permission
    ? `本会话始终允许此类（${permission.permission}）操作`
    : `本会话始终允许调用 ${name}`;

  return (
    <BottomSheet accessibilityLabel="权限确认" onDismiss={onMinimize} visible={visible && Boolean(permission)}>
      <View style={styles.head}>
        <View style={styles.avatarWrap}>
          <CharacterAvatar circular size={42} slot="attention" variant="avatar" />
          <View style={styles.warnBadge}>
            <Feather color={palette.warn} name="shield" size={11} />
          </View>
        </View>
        <View style={styles.headText}>
          <View style={styles.titleRow}>
            <PulseDot color={palette.warn} size={7} />
            <Text style={styles.title}>{permissionTitle(name)}</Text>
          </View>
          <Text style={styles.sub}>MyAI 请求在 {deviceID.trim() || "电脑"} 上执行</Text>
        </View>
      </View>

      {body ? (
        <View style={styles.cmd}>
          <Text selectable style={styles.cmdText}>
            {isShell ? <Text style={styles.prompt}>$ </Text> : null}
            {body}
          </Text>
        </View>
      ) : null}

      <View style={styles.kv}>
        <View style={styles.kvRow}>
          <Text style={styles.kvKey}>工具</Text>
          <Text style={styles.kvValue}>{name}{permission?.permission ? ` · ${permission.permission}` : ""}</Text>
        </View>
        {cwd ? (
          <View style={styles.kvRow}>
            <Text style={styles.kvKey}>目录</Text>
            <Text style={styles.kvValue}>{cwd}</Text>
          </View>
        ) : null}
      </View>

      <View style={styles.buttons}>
        <Pressable accessibilityRole="button" onPress={onDeny} style={({ pressed }) => [styles.button, styles.deny, pressed && styles.pressed]}>
          <Text style={styles.denyText}>拒绝本次</Text>
        </Pressable>
        <Pressable accessibilityRole="button" onPress={onAllow} style={({ pressed }) => [styles.button, styles.allow, pressed && styles.pressed]}>
          <Text style={styles.allowText}>{isShell ? "允许运行" : "允许本次"}</Text>
        </Pressable>
      </View>

      {permission?.requireFreshConsent ? (
        <Text style={styles.sub}>此操作需要单独确认。</Text>
      ) : onAllowSession ? (
        <Pressable
          accessibilityRole="button"
          onPress={onAllowSession}
          style={({ pressed }) => [styles.sessionAllowBtn, pressed && styles.pressed]}
        >
          <Feather color={palette.brandLight} name="check-circle" size={15} />
          <Text numberOfLines={1} style={styles.sessionAllowText}>{scopeLabel}</Text>
        </Pressable>
      ) : null}

      <Pressable accessibilityRole="button" onPress={onOpenPolicy} style={({ pressed }) => [styles.policy, pressed && styles.pressed]}>
        <Feather color={palette.ink3} name="shield" size={14} />
        <Text style={styles.policyText}>
          当前会话策略：<Text style={styles.policyValue}>{policyLabel}</Text> · 调整
        </Text>
      </Pressable>
    </BottomSheet>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    head: { alignItems: "center", flexDirection: "row", gap: 12 },
    avatarWrap: { height: 42, position: "relative", width: 42 },
    warnBadge: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.warn,
      borderRadius: radius.pill,
      borderWidth: 1,
      bottom: -2,
      height: 18,
      justifyContent: "center",
      position: "absolute",
      right: -2,
      width: 18,
    },
    headText: { flex: 1, minWidth: 0 },
    titleRow: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 7 },
    title: { color: palette.ink1, flexShrink: 1, fontSize: font.title, fontWeight: weight.semibold, lineHeight: 23 },
    sub: { color: palette.ink2, fontSize: font.label, marginTop: 2 },
    cmd: {
      backgroundColor: palette.well,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      marginTop: 14,
      paddingHorizontal: 14,
      paddingVertical: 12,
    },
    cmdText: { color: palette.ink1, fontFamily: fontFamily.mono, fontSize: font.label - 0.5, lineHeight: 21 },
    prompt: { color: palette.ink3 },
    kv: { gap: 6, marginTop: 12 },
    kvRow: { flexDirection: "row", gap: 14 },
    kvKey: { color: palette.ink3, fontSize: font.label - 0.5, minWidth: 28 },
    kvValue: { color: palette.ink1, flex: 1, fontFamily: fontFamily.mono, fontSize: font.caption, minWidth: 0 },
    buttons: { flexDirection: "row", gap: 10, marginTop: 18 },
    button: { alignItems: "center", borderRadius: radius.md, flex: 1, justifyContent: "center", minHeight: 46 },
    deny: { backgroundColor: palette.well, borderColor: palette.hairline, borderWidth: 1 },
    allow: { backgroundColor: palette.brand },
    denyText: { color: palette.ink1, fontSize: font.body, fontWeight: weight.semibold },
    allowText: { color: palette.inkInverse, fontSize: font.body, fontWeight: weight.bold },
    sessionAllowBtn: {
      alignItems: "center",
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      justifyContent: "center",
      marginTop: 10,
      minHeight: 42,
      paddingHorizontal: 12,
    },
    sessionAllowText: {
      color: palette.brandLight,
      flexShrink: 1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    pressed: { opacity: 0.8 },
    policy: { alignItems: "center", alignSelf: "center", flexDirection: "row", gap: 6, marginTop: 12, minHeight: 36, paddingHorizontal: 12 },
    policyText: { color: palette.ink3, fontSize: font.caption },
    policyValue: { color: palette.ink2 },
  });
