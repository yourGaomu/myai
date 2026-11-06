import { useEffect, useRef } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ContextInfo, ModelSummary, SessionSummary } from "../../protocol";
import type { SessionAgentMode, SessionPermissionMode } from "../../types/app";
import { modelDisplayName, modelProviderLabel, normalizePermission } from "../../utils/session";
import { formatTokenCount } from "../../utils/tokenUsage";
import { fontFamily, font, palette, radius, weight } from "../../constants/theme";
import { BottomSheet } from "../common/BottomSheet";
import { Segmented } from "../common/Segmented";

type Props = {
  activeSession?: SessionSummary;
  connected: boolean;
  connectionLabel: string;
  context?: ContextInfo;
  currentModelID: string;
  models: ModelSummary[];
  onClose: () => void;
  onCompact: () => void;
  onReconnect: () => void;
  onRequestContext: () => void;
  onSetAgentMode: (mode: SessionAgentMode) => void;
  onSetPermissionMode: (mode: SessionPermissionMode) => void;
  onSwitchModel: (modelID: string) => void;
  pendingModels: boolean;
  pendingSettings: boolean;
  visible: boolean;
};

const permissionHints: Record<SessionPermissionMode, string> = {
  readonly: "只允许读取文件和搜索，不会修改文件或执行命令",
  ask: "运行命令等危险操作前先问你",
  full: "所有工具调用自动执行，不再询问",
};

// 会话设置：点顶部标题或输入框里的模型打开。模型、模式、工具权限、上下文集中在这里。
export function SessionSheet(props: Props) {
  const { activeSession, context, visible } = props;
  const hasSession = Boolean(activeSession?.id);
  const mode: SessionAgentMode = activeSession?.agent_mode === "plan" ? "plan" : "chat";
  const permission = normalizePermission(activeSession?.permission_mode);
  const models = props.models.filter((model) => model.enabled !== false);
  const windowK = context?.window_k || activeSession?.context_window_k || 0;
  const used = context?.selected_tokens ?? activeSession?.last_usage?.prompt_tokens;
  const ratio = windowK && used ? Math.min(1, used / (windowK * 1000)) : 0;
  const settingsDisabled = !hasSession || !props.connected || props.pendingSettings;
  const { connected } = props;
  // 回调每次渲染都可能是新引用，用 ref 避免弹层打开期间反复请求上下文
  const requestContextRef = useRef(props.onRequestContext);
  requestContextRef.current = props.onRequestContext;

  useEffect(() => {
    if (visible && connected && hasSession) {
      requestContextRef.current();
    }
  }, [connected, hasSession, visible]);

  return (
    <BottomSheet accessibilityLabel="会话设置" onDismiss={props.onClose} visible={visible}>
      <View style={styles.head}>
        <Text numberOfLines={1} style={styles.title}>{activeSession?.title?.trim() || "新对话"}</Text>
        <Pressable accessibilityLabel="关闭" hitSlop={8} onPress={props.onClose} style={({ pressed }) => [styles.close, pressed && styles.pressed]}>
          <Feather color={palette.ink2} name="x" size={20} />
        </Pressable>
      </View>

      {!props.connected ? (
        <View style={styles.offline}>
          <View style={styles.offlineDot} />
          <Text style={styles.offlineText}>{props.connectionLabel}</Text>
          <Pressable accessibilityRole="button" hitSlop={8} onPress={props.onReconnect}>
            <Text style={styles.link}>重新连接</Text>
          </Pressable>
        </View>
      ) : null}

      <Text style={styles.label}>模型</Text>
      <View style={styles.group}>
        {models.length === 0 ? <Text style={styles.empty}>还没有可用模型，到「设置 › 模型」里添加</Text> : null}
        {models.map((model, index) => {
          const checked = model.id === props.currentModelID;
          const sub = [modelProviderLabel(model), model.is_default ? "默认" : ""].filter(Boolean).join(" · ");
          return (
            <Pressable
              accessibilityRole="radio"
              accessibilityState={{ checked, disabled: props.pendingModels }}
              disabled={props.pendingModels || !props.connected}
              key={model.id}
              onPress={() => props.onSwitchModel(model.id)}
              style={({ pressed }) => [styles.radio, index > 0 && styles.radioDivider, pressed && styles.pressedRow]}
            >
              <View style={styles.radioText}>
                <Text numberOfLines={1} style={styles.radioTitle}>{modelDisplayName(model)}</Text>
                {sub ? <Text numberOfLines={1} style={styles.radioSub}>{sub}</Text> : null}
              </View>
              {checked ? (
                props.pendingModels ? <ActivityIndicator color={palette.brand} size="small" /> : <Feather color={palette.brandLight} name="check" size={18} />
              ) : null}
            </Pressable>
          );
        })}
      </View>

      <Text style={styles.label}>模式</Text>
      <Segmented
        accessibilityLabel="模式"
        disabled={settingsDisabled}
        onChange={props.onSetAgentMode}
        options={[{ key: "chat", label: "对话" }, { key: "plan", label: "规划" }]}
        value={mode}
      />

      <Text style={styles.label}>工具权限</Text>
      <Segmented
        accessibilityLabel="工具权限"
        disabled={settingsDisabled}
        onChange={props.onSetPermissionMode}
        options={[{ key: "readonly", label: "只读" }, { key: "ask", label: "询问" }, { key: "full", label: "完全开放" }]}
        value={permission}
      />
      <Text style={[styles.hint, permission === "full" && styles.hintWarn]}>{permissionHints[permission]}</Text>

      <Text style={styles.label}>上下文</Text>
      <View style={[styles.group, styles.contextRow]}>
        <View style={styles.bar}>
          <View style={[styles.barFill, { width: `${Math.round(ratio * 100)}%` }]} />
        </View>
        <Text style={styles.contextValue}>
          {used !== undefined ? formatTokenCount(used) : "—"} / {windowK ? `${windowK}K` : "—"}
        </Text>
        <Pressable accessibilityRole="button" disabled={settingsDisabled} hitSlop={6} onPress={props.onCompact} style={settingsDisabled && styles.dimmed}>
          <Text style={styles.link}>压缩</Text>
        </Pressable>
      </View>
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  head: { alignItems: "center", flexDirection: "row", gap: 8, marginBottom: 2 },
  title: { color: palette.ink1, flex: 1, fontSize: font.title, fontWeight: weight.semibold },
  close: { alignItems: "center", borderRadius: radius.pill, height: 40, justifyContent: "center", width: 40 },
  pressed: { opacity: 0.7 },
  pressedRow: { backgroundColor: palette.pressed },
  dimmed: { opacity: 0.4 },
  offline: { alignItems: "center", backgroundColor: palette.errWash, borderRadius: radius.md - 2, flexDirection: "row", gap: 8, marginTop: 10, minHeight: 44, paddingHorizontal: 12 },
  offlineDot: { backgroundColor: palette.err, borderRadius: radius.pill, height: 7, width: 7 },
  offlineText: { color: palette.ink1, flex: 1, fontSize: font.label },
  label: { color: palette.ink3, fontSize: font.label - 0.5, fontWeight: weight.medium, marginBottom: 8, marginLeft: 2, marginTop: 16 },
  group: { backgroundColor: palette.well, borderRadius: radius.lg, overflow: "hidden" },
  empty: { color: palette.ink3, fontSize: font.label, padding: 14 },
  radio: { alignItems: "center", flexDirection: "row", gap: 12, minHeight: 52, paddingHorizontal: 14, paddingVertical: 6 },
  radioDivider: { borderTopColor: palette.hairline, borderTopWidth: 1 },
  radioText: { flex: 1, minWidth: 0 },
  radioTitle: { color: palette.ink1, fontSize: font.body },
  radioSub: { color: palette.ink3, fontSize: font.caption - 0.5, marginTop: 1 },
  hint: { color: palette.ink3, fontSize: font.caption, marginLeft: 2, marginTop: 8 },
  hintWarn: { color: palette.warn },
  contextRow: { alignItems: "center", flexDirection: "row", gap: 12, minHeight: 50, paddingLeft: 14, paddingRight: 14 },
  bar: { backgroundColor: palette.hairlineStrong, borderRadius: radius.xs, flex: 1, height: 6, overflow: "hidden" },
  barFill: { backgroundColor: palette.brand, borderRadius: radius.xs, height: "100%" },
  contextValue: { color: palette.ink2, fontFamily: fontFamily.mono, fontSize: font.caption },
  link: { color: palette.brandLight, fontSize: font.label, fontWeight: weight.medium },
});
