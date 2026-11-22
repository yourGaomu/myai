import { useEffect, useRef } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { ContextInfo, ModelSummary, SessionSummary } from "../../protocol";
import type { SessionAgentMode, SessionPermissionMode } from "../../types/app";
import { modelDisplayName, modelProviderLabel, normalizePermission } from "../../utils/session";
import { describeSessionContext, formatContextUsage } from "../../utils/sessionContext";
import { fontFamily, font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { BottomSheet } from "../common/BottomSheet";
import { ShimmerProgressBar } from "../common/MotionEffects";
import { Segmented } from "../common/Segmented";

type Props = {
  activeSession?: SessionSummary;
  connected: boolean;
  connectionLabel: string;
  context?: ContextInfo;
  contextFirst: boolean;
  pendingContext: boolean;
  sessionID: string;
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

const permissionModes: Array<{ key: SessionPermissionMode; label: string }> = [
  { key: "readonly", label: "只读" },
  { key: "ask", label: "询问" },
  { key: "read_write", label: "仅读写" },
  { key: "execute", label: "可执行" },
  { key: "full", label: "全开放" },
];

const permissionHints: Record<SessionPermissionMode, string> = {
  readonly: "仅允许读取与检索，严禁写入或执行命令",
  ask: "读写和命令行等变更操作前先征询你",
  read_write: "允许读写文件，执行终端命令仍需你授权",
  execute: "允许执行命令，高危边界仍受系统校验",
  full: "所有工具调用自动执行，不再弹窗中断",
};

// 会话设置：模型、模式、细粒度工具权限（完全对齐后端 5 档策略）、上下文压缩。
export function SessionSheet(props: Props) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const { activeSession, context, sessionID, visible } = props;
  const hasSession = Boolean(sessionID);
  const mode: SessionAgentMode = activeSession?.agent_mode === "plan" ? "plan" : "chat";
  const permission = normalizePermission(activeSession?.permission_mode);
  const models = props.models.filter((model) => model.enabled !== false);
  const { sample, categories, used, window, snapshotUsed, snapshotWindow } = describeSessionContext(context, activeSession?.context_window_k);
  const ratio = window && used !== undefined ? Math.min(1, used / window) : 0;
  const settingsDisabled = !hasSession || !props.connected || props.pendingSettings;
  const { connected } = props;
  const requestContextRef = useRef(props.onRequestContext);
  requestContextRef.current = props.onRequestContext;

  useEffect(() => {
    if (visible && connected && hasSession) {
      requestContextRef.current();
    }
  }, [connected, sessionID, visible]);

  const contextDetails = hasSession ? (
    <>
      <Text accessibilityRole="header" style={styles.label}>上下文</Text>
      <View style={[styles.group, styles.context]}>
        <View style={styles.contextRow}>
          <Text style={styles.contextTitle}>{sample ? "最近一次成功的模型请求" : "当前会话快照"}</Text>
          <Pressable accessibilityRole="button" disabled={settingsDisabled} hitSlop={6} onPress={props.onCompact} style={settingsDisabled && styles.dimmed}>
            <Text style={styles.link}>压缩上下文</Text>
          </Pressable>
        </View>
        {sample?.model ? <Text style={styles.contextNote}>{sample.model}</Text> : null}
        <Text style={styles.contextValue}>估算输入 / 窗口（令牌）  {formatContextUsage(used, window, true)}</Text>
        <ShimmerProgressBar
          active={ratio >= 0.8}
          height={6}
          progress={Math.round(ratio * 100)}
          tone={ratio >= 0.8 ? "warn" : "brand"}
        />
        {sample ? (
          <>
            <View style={styles.contextBreakdown}>
              {categories.map(({ label, tokens }) => (
                <View key={label} style={styles.contextLine}>
                  <Text style={styles.contextNote}>{label}</Text>
                  <Text style={styles.contextValue}>{tokens.toLocaleString()}</Text>
                </View>
              ))}
            </View>
            <View style={styles.contextBreakdown}>
              <View style={styles.contextLine}>
                <Text style={styles.contextNote}>输出预留</Text>
                <Text style={styles.contextValue}>{sample.output_reserve_tokens.toLocaleString()}</Text>
              </View>
              <View style={styles.contextLine}>
                <Text style={styles.contextNote}>安全余量</Text>
                <Text style={styles.contextValue}>{sample.safety_reserve_tokens.toLocaleString()}</Text>
              </View>
            </View>
            <Text style={styles.contextNote}>输入为本地估算（含图片和工具定义）。两项预留占用窗口容量，单独列出。</Text>
            {snapshotUsed !== undefined ? (
              <Text style={styles.contextNote}>当前会话快照（估算）  {formatContextUsage(snapshotUsed, snapshotWindow, true)}</Text>
            ) : null}
          </>
        ) : (
          <Text style={styles.contextNote}>
            {props.pendingContext ? "正在读取上下文…" : "暂无最近一次模型请求的分类及预留数据。用量采用会话快照的本地估算。"}
          </Text>
        )}
      </View>
    </>
  ) : null;

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

      {props.contextFirst ? contextDetails : null}

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

      <Text style={styles.label}>工具权限（切换至谨慎/只读会清除本会话临时免询规则）</Text>
      <View style={styles.permWrap}>
        {permissionModes.map((item) => {
          const selected = permission === item.key;
          return (
            <Pressable
              accessibilityRole="radio"
              accessibilityState={{ checked: selected, disabled: settingsDisabled }}
              disabled={settingsDisabled}
              key={item.key}
              onPress={() => {
                if (!selected) props.onSetPermissionMode(item.key);
              }}
              style={({ pressed }) => [
                styles.permChip,
                selected && styles.permChipActive,
                settingsDisabled && styles.dimmed,
                pressed && styles.pressed,
              ]}
            >
              <Text style={[styles.permChipText, selected && styles.permChipTextActive]}>{item.label}</Text>
            </Pressable>
          );
        })}
      </View>
      <Text style={[styles.hint, permission === "full" && styles.hintWarn]}>{permissionHints[permission]}</Text>

      {!props.contextFirst ? contextDetails : null}
    </BottomSheet>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    head: { alignItems: "center", flexDirection: "row", gap: 8, marginBottom: 2 },
    title: { color: palette.ink1, flex: 1, fontSize: font.title, fontWeight: weight.semibold },
    close: { alignItems: "center", borderRadius: radius.pill, height: 40, justifyContent: "center", width: 40 },
    pressed: { opacity: 0.72 },
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
    permWrap: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
    permChip: {
      alignItems: "center",
      backgroundColor: palette.well,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      minHeight: 34,
      justifyContent: "center",
      paddingHorizontal: 12,
    },
    permChipActive: {
      backgroundColor: palette.brandWashStrong,
      borderColor: palette.brand,
    },
    permChipText: { color: palette.ink2, fontSize: font.label - 0.5, fontWeight: weight.medium },
    permChipTextActive: { color: palette.brandLight, fontWeight: weight.semibold },
    hint: { color: palette.ink3, fontSize: font.caption, marginLeft: 2, marginTop: 8 },
    hintWarn: { color: palette.warn },
    context: { gap: 12, padding: 14 },
    contextRow: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", columnGap: 12, rowGap: 8, justifyContent: "space-between" },
    contextLine: { alignItems: "baseline", flexDirection: "row", gap: 8, justifyContent: "space-between" },
    contextBreakdown: { borderTopColor: palette.hairline, borderTopWidth: 1, gap: 5, paddingTop: 8 },
    contextNote: { color: palette.ink3, fontSize: font.caption },
    contextTitle: { color: palette.ink1, flexShrink: 1, fontSize: font.label, fontWeight: weight.semibold },
    contextValue: { color: palette.ink2, flexShrink: 1, fontFamily: fontFamily.mono, fontSize: font.caption },
    link: { color: palette.brandLight, fontSize: font.label, fontWeight: weight.medium },
  });
