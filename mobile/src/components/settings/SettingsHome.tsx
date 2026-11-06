import type { ComponentProps } from "react";
import { Fragment } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, palette, radius, weight } from "../../constants/theme";

export type SettingsSection =
  | "general"
  | "connection"
  | "model"
  | "autoplan"
  | "skill"
  | "plugin"
  | "subagent"
  | "session"
  | "permission"
  | "context"
  | "generation";

export type SettingsRow = {
  icon: ComponentProps<typeof Feather>["name"];
  key: SettingsSection;
  label: string;
  value?: string;
};

type Props = {
  connected: boolean;
  deviceID: string;
  groups: { label: string; rows: SettingsRow[] }[];
  onOpenSection: (section: SettingsSection) => void;
  relayHost: string;
  versionLabel: string;
};

export const settingsSectionTitles: Record<SettingsSection, string> = {
  autoplan: "自动规划",
  connection: "连接与配对",
  context: "上下文",
  general: "版本与热更新",
  generation: "生成参数与回复风格",
  model: "模型",
  permission: "工具权限",
  plugin: "插件与 MCP",
  session: "会话管理",
  skill: "技能",
  subagent: "子代理",
};

// 设置首页：设备卡片 + 分组列表，点一行进入对应详情。
export function SettingsHome({ connected, deviceID, groups, onOpenSection, relayHost, versionLabel }: Props) {
  return (
    <View>
      <Pressable onPress={() => onOpenSection("connection")} style={({ pressed }) => [styles.device, pressed && styles.pressed]}>
        <View style={styles.deviceIcon}>
          <Feather color={palette.brandLight} name="monitor" size={22} />
        </View>
        <View style={styles.flex}>
          <Text numberOfLines={1} style={styles.deviceName}>{deviceID.trim() || "电脑"}</Text>
          <View style={styles.deviceStatus}>
            <View style={[styles.dot, !connected && styles.dotOff]} />
            <Text numberOfLines={1} style={styles.deviceMeta}>{connected ? "在线" : "离线"} · {relayHost || "未设置中继"}</Text>
          </View>
        </View>
        <Feather color={palette.ink4} name="chevron-right" size={16} />
      </Pressable>

      {groups.map((group) => (
        <Fragment key={group.label}>
          <Text style={styles.groupLabel}>{group.label}</Text>
          <View style={styles.group}>
            {group.rows.map((row, index) => (
              <Pressable
                accessibilityRole="button"
                key={row.key}
                onPress={() => onOpenSection(row.key)}
                style={({ pressed }) => [styles.row, pressed && styles.pressed]}
              >
                {index > 0 ? <View style={styles.divider} /> : null}
                <Feather color={palette.ink2} name={row.icon} size={19} />
                <Text numberOfLines={1} style={styles.rowLabel}>{row.label}</Text>
                {row.value ? <Text numberOfLines={1} style={styles.rowValue}>{row.value}</Text> : null}
                <Feather color={palette.ink4} name="chevron-right" size={16} />
              </Pressable>
            ))}
          </View>
        </Fragment>
      ))}

      <Text style={styles.version}>MyAI Mobile {versionLabel}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1, minWidth: 0 },
  pressed: { backgroundColor: palette.pressed },
  device: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.lg + 2,
    borderWidth: 1,
    flexDirection: "row",
    gap: 12,
    padding: 14,
  },
  deviceIcon: { alignItems: "center", backgroundColor: palette.brandWash, borderRadius: radius.md, height: 44, justifyContent: "center", width: 44 },
  deviceName: { color: palette.ink1, fontSize: font.bodyLg, fontWeight: weight.semibold },
  deviceStatus: { alignItems: "center", flexDirection: "row", gap: 6, marginTop: 2 },
  dot: { backgroundColor: palette.ok, borderRadius: radius.pill, height: 7, width: 7 },
  dotOff: { backgroundColor: palette.err },
  deviceMeta: { color: palette.ink2, flexShrink: 1, fontSize: font.caption },
  groupLabel: { color: palette.ink3, fontSize: font.label - 0.5, fontWeight: weight.medium, paddingBottom: 7, paddingLeft: 16, paddingTop: 22 },
  group: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.lg + 2,
    borderWidth: 1,
    overflow: "hidden",
  },
  row: { alignItems: "center", flexDirection: "row", gap: 12, minHeight: 50, paddingHorizontal: 14 },
  divider: { backgroundColor: palette.hairline, height: 1, left: 45, position: "absolute", right: 0, top: 0 },
  rowLabel: { color: palette.ink1, flex: 1, fontSize: font.body },
  rowValue: { color: palette.ink3, fontSize: font.label + 0.5, maxWidth: "46%" },
  version: { color: palette.ink4, fontSize: font.caption, paddingBottom: 8, paddingTop: 22, textAlign: "center" },
});
