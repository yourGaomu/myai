import { Pressable, StyleSheet, Switch, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { PluginInfo } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, weight } from "../../constants/theme";

type Props = {
  buttonFeedback: ButtonFeedback;
  message: string;
  onRefresh: () => void;
  onReload: () => void;
  onSetEnabled: (pluginID: string, enabled: boolean) => void;
  pending: boolean;
  plugins: PluginInfo[];
  root: string;
};

export function PluginPanel({ buttonFeedback, message, onRefresh, onReload, onSetEnabled, pending, plugins, root }: Props) {
  return (
    <View style={styles.stack}>
      <View style={styles.headerCard}>
        <View style={styles.flex}>
          <Text style={styles.title}>插件</Text>
          <Text numberOfLines={2} style={styles.meta}>
            {plugins.length} 个插件{root ? ` / ${root}` : ""}
          </Text>
        </View>
        <View style={styles.row}>
          <Pressable disabled={pending} onPress={onRefresh} style={({ pressed }) => buttonFeedback([styles.action, pending && styles.disabled], pressed)}>
            <ButtonContent color={palette.ink1} loading={pending} text={pending ? "加载中" : "刷新"} />
          </Pressable>
          <Pressable disabled={pending} onPress={onReload} style={({ pressed }) => buttonFeedback([styles.primary, pending && styles.disabled], pressed)}>
            <ButtonContent loading={pending} text={pending ? "重载中" : "重载"} />
          </Pressable>
        </View>
      </View>

      {message ? <Text style={styles.message}>{message}</Text> : null}
      {plugins.length === 0 ? (
        <Text style={styles.empty}>还没有发现本地插件。将插件目录放到 Agent 工作区的 plugins 下即可。</Text>
      ) : plugins.map((plugin) => {
        const failed = plugin.status === "failed";
        return (
          <View key={plugin.id} style={styles.card}>
            <View style={styles.flex}>
              <View style={styles.nameRow}>
                <Text numberOfLines={1} style={styles.name}>{plugin.name || plugin.id}</Text>
                <Text style={[styles.badge, failed && styles.badgeFailed]}>{plugin.status}</Text>
              </View>
              <Text style={styles.meta}>{plugin.id} · v{plugin.version || "0.1.0"} · {plugin.protocol || "mcp"}</Text>
              {plugin.error ? <Text numberOfLines={3} style={styles.error}>{plugin.error}</Text> : null}
            </View>
            <Switch
              disabled={pending}
              onValueChange={(value) => onSetEnabled(plugin.id, value)}
              value={plugin.enabled}
              trackColor={{ false: palette.surface4, true: palette.brand }}
              thumbColor={palette.ink1}
            />
          </View>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  stack: { gap: 10 },
  headerCard: { alignItems: "center", backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, flexDirection: "row", gap: 10, padding: 12 },
  card: { alignItems: "center", backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, flexDirection: "row", gap: 10, padding: 12 },
  flex: { flex: 1, minWidth: 0 },
  title: { color: palette.ink1, fontSize: font.title, fontWeight: weight.semibold },
  name: { color: palette.ink1, flexShrink: 1, fontSize: font.body, fontWeight: weight.semibold },
  meta: { color: palette.ink2, fontSize: font.label, marginTop: 3 },
  nameRow: { alignItems: "center", flexDirection: "row", gap: 8 },
  badge: { backgroundColor: palette.okWash, borderRadius: radius.sm, color: palette.ok, fontSize: font.micro, paddingHorizontal: 6, paddingVertical: 2, textTransform: "uppercase" },
  badgeFailed: { backgroundColor: palette.errWash, color: palette.err },
  error: { color: palette.err, fontSize: font.label, marginTop: 5 },
  row: { flexDirection: "row", gap: 6 },
  action: { backgroundColor: palette.well, borderColor: palette.hairline, borderRadius: radius.sm, borderWidth: 1, minWidth: 54, paddingHorizontal: 9, paddingVertical: 7 },
  primary: { backgroundColor: palette.brand, borderRadius: radius.sm, minWidth: 54, paddingHorizontal: 9, paddingVertical: 7 },
  disabled: { opacity: 0.5 },
  message: { color: palette.ink3, fontSize: font.label },
  empty: { backgroundColor: palette.well, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, color: palette.ink2, fontSize: font.body, padding: 14 },
});
