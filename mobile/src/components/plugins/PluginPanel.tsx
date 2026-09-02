import { Pressable, StyleSheet, Switch, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { PluginInfo } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";

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
            <ButtonContent loading={pending} text={pending ? "加载中" : "刷新"} />
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
              trackColor={{ false: "#4b4741", true: "#8f6b3f" }}
              thumbColor={plugin.enabled ? "#f5eee4" : "#bdb4a8"}
            />
          </View>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  stack: { gap: 10 },
  headerCard: { alignItems: "center", backgroundColor: "#211f1c", borderColor: "#403b35", borderRadius: 14, borderWidth: 1, flexDirection: "row", gap: 10, padding: 12 },
  card: { alignItems: "center", backgroundColor: "#1c1a18", borderColor: "#36322d", borderRadius: 12, borderWidth: 1, flexDirection: "row", gap: 10, padding: 12 },
  flex: { flex: 1, minWidth: 0 },
  title: { color: "#f4eee5", fontSize: 16, fontWeight: "700" },
  name: { color: "#f4eee5", flexShrink: 1, fontSize: 14, fontWeight: "700" },
  meta: { color: "#a9a096", fontSize: 12, marginTop: 3 },
  nameRow: { alignItems: "center", flexDirection: "row", gap: 8 },
  badge: { backgroundColor: "#2e4b3b", borderRadius: 6, color: "#b9e0c2", fontSize: 10, paddingHorizontal: 6, paddingVertical: 2, textTransform: "uppercase" },
  badgeFailed: { backgroundColor: "#52302d", color: "#f3b9b1" },
  error: { color: "#f0a59a", fontSize: 12, marginTop: 5 },
  row: { flexDirection: "row", gap: 6 },
  action: { borderColor: "#5d554c", borderRadius: 8, borderWidth: 1, minWidth: 54, paddingHorizontal: 9, paddingVertical: 7 },
  primary: { backgroundColor: "#8f6b3f", borderRadius: 8, minWidth: 54, paddingHorizontal: 9, paddingVertical: 7 },
  disabled: { opacity: 0.5 },
  message: { color: "#c9bda9", fontSize: 12 },
  empty: { backgroundColor: "#1c1a18", borderColor: "#36322d", borderRadius: 10, borderWidth: 1, color: "#91887e", fontSize: 13, padding: 14 },
});
