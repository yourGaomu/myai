import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { PermissionState } from "../../types/app";
import { permissionBody, permissionTitle, toolCwd, toolKind } from "../../utils/toolDisplay";
import { fontFamily, font, palette, radius, weight } from "../../constants/theme";
import { BottomSheet } from "../common/BottomSheet";

type Props = {
  deviceID: string;
  onAllow: () => void;
  onDeny: () => void;
  // 点遮罩 / 返回键：收成输入框上方的提示条，请求保持待确认
  onMinimize: () => void;
  onOpenPolicy: () => void;
  permission: PermissionState | null;
  policyLabel: string;
  visible: boolean;
};

// 工具调用权限确认：完整展示命令、工具和目录，允许 / 拒绝两个大按钮。
export function PermissionSheet({ deviceID, onAllow, onDeny, onMinimize, onOpenPolicy, permission, policyLabel, visible }: Props) {
  const name = permission?.name || "";
  const body = permission ? permissionBody(name, permission.arguments) : "";
  const cwd = permission ? toolCwd(permission.arguments) : "";
  const isShell = toolKind(name) === "shell";

  return (
    <BottomSheet accessibilityLabel="权限确认" onDismiss={onMinimize} visible={visible && Boolean(permission)}>
      <View style={styles.head}>
        <View style={styles.icon}>
          <Feather color={palette.warn} name="shield" size={20} />
        </View>
        <View style={styles.headText}>
          <Text style={styles.title}>{permissionTitle(name)}</Text>
          <Text style={styles.sub}>MyAI 想在 {deviceID.trim() || "电脑"} 上执行</Text>
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
          <Text style={styles.denyText}>拒绝</Text>
        </Pressable>
        <Pressable accessibilityRole="button" onPress={onAllow} style={({ pressed }) => [styles.button, styles.allow, pressed && styles.pressed]}>
          <Text style={styles.allowText}>{isShell ? "允许运行" : "允许"}</Text>
        </Pressable>
      </View>

      <Pressable accessibilityRole="button" onPress={onOpenPolicy} style={({ pressed }) => [styles.policy, pressed && styles.pressed]}>
        <Feather color={palette.ink3} name="shield" size={14} />
        <Text style={styles.policyText}>
          当前策略：<Text style={styles.policyValue}>{policyLabel}</Text> · 修改
        </Text>
      </Pressable>
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  head: { alignItems: "flex-start", flexDirection: "row", gap: 12 },
  icon: { alignItems: "center", backgroundColor: palette.warnWash, borderRadius: radius.sm + 2, height: 40, justifyContent: "center", width: 40 },
  headText: { flex: 1 },
  title: { color: palette.ink1, fontSize: font.title, fontWeight: weight.semibold, lineHeight: 23 },
  sub: { color: palette.ink2, fontSize: font.label, marginTop: 2 },
  cmd: { backgroundColor: palette.well, borderRadius: radius.md, marginTop: 16, paddingHorizontal: 14, paddingVertical: 12 },
  cmdText: { color: palette.ink1, fontFamily: fontFamily.mono, fontSize: font.label - 0.5, lineHeight: 21 },
  prompt: { color: palette.ink3 },
  kv: { gap: 6, marginTop: 12 },
  kvRow: { flexDirection: "row", gap: 14 },
  kvKey: { color: palette.ink3, fontSize: font.label - 0.5, minWidth: 28 },
  kvValue: { color: palette.ink1, flex: 1, fontFamily: fontFamily.mono, fontSize: font.caption },
  buttons: { flexDirection: "row", gap: 10, marginTop: 20 },
  button: { alignItems: "center", borderRadius: radius.md, flex: 1, justifyContent: "center", minHeight: 48 },
  deny: { backgroundColor: palette.well },
  allow: { backgroundColor: palette.brand },
  denyText: { color: palette.ink1, fontSize: font.body, fontWeight: weight.semibold },
  allowText: { color: palette.inkInverse, fontSize: font.body, fontWeight: weight.semibold },
  pressed: { opacity: 0.8 },
  policy: { alignItems: "center", flexDirection: "row", gap: 6, justifyContent: "center", marginTop: 10, minHeight: 40 },
  policyText: { color: palette.ink3, fontSize: font.label - 0.5 },
  policyValue: { color: palette.brandLight, fontWeight: weight.medium },
});
