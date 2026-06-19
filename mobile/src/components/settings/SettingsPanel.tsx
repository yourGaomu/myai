import { useEffect, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { ContextInfo, ModelSummary, SessionSummary } from "../../protocol";
import type { PendingAction, SessionPermissionMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { shortID } from "../../utils/ids";
import { websocketURL } from "../../utils/relay";
import { modelDisplayName } from "../../utils/session";

type Props = {
  activeModel?: ModelSummary;
  activeSession?: SessionSummary;
  bindCode: string;
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  connected: boolean;
  context?: ContextInfo;
  currentModelID: string;
  deviceID: string;
  models: ModelSummary[];
  normalizedRelayURL: string;
  onBindCodeChange: (value: string) => void;
  onClose: () => void;
  onCompactSession: () => void;
  onConnect: () => void;
  onDeviceIDChange: (value: string) => void;
  onLoadSession: (sessionID: string) => void;
  onNewSession: () => void;
  onPair: () => void;
  onRefreshModels: () => void;
  onRefreshSessions: () => void;
  onRelayURLChange: (value: string) => void;
  onSetContextWindowK: (windowK: number) => void;
  onSetPermissionMode: (mode: SessionPermissionMode) => void;
  onSwitchModel: (modelID: string) => void;
  onUserIDChange: (value: string) => void;
  pendingActions: Record<PendingAction, boolean>;
  relayURL: string;
  sessionID: string;
  sessions: SessionSummary[];
  userID: string;
};

const permissionModes: Array<{ label: string; mode: SessionPermissionMode; meta: string }> = [
  { label: "Read", mode: "readonly", meta: "no write/run" },
  { label: "Ask", mode: "ask", meta: "confirm tools" },
  { label: "Full", mode: "full", meta: "auto allow" },
];
const contextPresets = [8, 16, 32, 64, 128];

export function SettingsPanel({
  activeModel,
  activeSession,
  bindCode,
  buttonFeedback,
  clientToken,
  connected,
  context,
  currentModelID,
  deviceID,
  models,
  normalizedRelayURL,
  onBindCodeChange,
  onClose,
  onCompactSession,
  onConnect,
  onDeviceIDChange,
  onLoadSession,
  onNewSession,
  onPair,
  onRefreshModels,
  onRefreshSessions,
  onRelayURLChange,
  onSetContextWindowK,
  onSetPermissionMode,
  onSwitchModel,
  onUserIDChange,
  pendingActions,
  relayURL,
  sessionID,
  sessions,
  userID,
}: Props) {
  const activePermission = normalizePermissionMode(activeSession?.permission_mode);
  const currentWindowK = context?.window_k || activeSession?.context_window_k || 16;
  const [windowInput, setWindowInput] = useState(String(currentWindowK));

  useEffect(() => {
    setWindowInput(String(currentWindowK));
  }, [currentWindowK]);

  const settingsBusy = pendingActions.settings;
  const canUseSessionSettings = Boolean(clientToken && sessionID);
  const submitWindow = () => {
    const nextWindowK = Number.parseInt(windowInput, 10);
    if (Number.isNaN(nextWindowK)) {
      return;
    }
    onSetContextWindowK(nextWindowK);
  };

  return (
    <View style={[styles.panel, styles.settingsPanel]}>
      <View style={styles.panelHeader}>
        <View style={styles.flex}>
          <Text style={styles.settingsTitle}>Settings</Text>
          <Text style={styles.pathText}>Connection, model, session controls</Text>
        </View>
        <Pressable onPress={onClose} style={({ pressed }) => buttonFeedback(styles.smallButton, pressed)}>
          <Text style={styles.smallButtonText}>Done</Text>
        </Pressable>
      </View>

      <View style={styles.settingCard}>
        <IconBox label="WS" />
        <View style={styles.flex}>
          <Text style={styles.settingTitle}>Relay</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>{websocketURL(normalizedRelayURL)}</Text>
        </View>
        <Pressable
          disabled={pendingActions.connect}
          onPress={onConnect}
          style={({ pressed }) =>
            buttonFeedback(
              [
                styles.settingAction,
                connected ? styles.settingActionOnline : styles.settingActionWarn,
                pendingActions.connect && styles.disabledButton,
              ],
              pressed,
            )
          }
        >
          <ButtonContent loading={pendingActions.connect} text={pendingActions.connect ? "Connecting" : connected ? "Online" : "Connect"} />
        </Pressable>
      </View>

      <TextInput
        autoCapitalize="none"
        autoCorrect={false}
        onChangeText={onRelayURLChange}
        placeholder="http://server:18080"
        placeholderTextColor="#776f66"
        style={styles.input}
        value={relayURL}
      />
      <View style={styles.row}>
        <TextInput
          keyboardType="number-pad"
          maxLength={6}
          onChangeText={onBindCodeChange}
          placeholder="Bind code"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.flex]}
          value={bindCode}
        />
        <Pressable
          disabled={pendingActions.pair}
          onPress={onPair}
          style={({ pressed }) => buttonFeedback([styles.secondaryButton, pendingActions.pair && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.pair} text={pendingActions.pair ? "Pairing" : "Pair"} />
        </Pressable>
      </View>
      <View style={styles.row}>
        <TextInput
          onChangeText={onUserIDChange}
          placeholder="User"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.flex]}
          value={userID}
        />
        <TextInput
          onChangeText={onDeviceIDChange}
          placeholder="Device"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.flex]}
          value={deviceID}
        />
      </View>

      <View style={styles.settingCard}>
        <IconBox label="AI" />
        <View style={styles.flex}>
          <Text style={styles.settingTitle}>Model</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>
            {activeModel ? modelDisplayName(activeModel) : currentModelID || activeSession?.model || "No model loaded"}
          </Text>
        </View>
        <Pressable
          disabled={pendingActions.models}
          onPress={onRefreshModels}
          style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.models && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.models} text={pendingActions.models ? "Loading" : "Refresh"} />
        </Pressable>
      </View>

      {models.length > 0 ? (
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <View style={styles.modelRow}>
            {models.map((model) => {
              const selected = model.id === currentModelID;
              const disabled = model.enabled === false;
              return (
                <Pressable
                  disabled={disabled || selected || pendingActions.models}
                  key={model.id}
                  onPress={() => onSwitchModel(model.id)}
                  style={({ pressed }) =>
                    buttonFeedback([styles.modelChip, selected && styles.modelChipActive, disabled && styles.disabledButton], pressed)
                  }
                >
                  <Text style={styles.modelTitle}>{modelDisplayName(model)}</Text>
                  <Text style={styles.modelMeta}>
                    {model.provider || "provider"} / {model.model_name || model.id}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </ScrollView>
      ) : (
        <Text style={styles.emptyText}>{clientToken ? "No models loaded" : "Pair first"}</Text>
      )}

      <View style={styles.settingCard}>
        <IconBox label="S" />
        <View style={styles.flex}>
          <Text style={styles.settingTitle}>Session</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>
            {activeSession?.title || (sessionID ? shortID(sessionID) : "No session selected")}
          </Text>
        </View>
        <View style={styles.rowCompact}>
          <Pressable
            disabled={pendingActions.sessions}
            onPress={onNewSession}
            style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.sessions && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingActions.sessions} text={pendingActions.sessions ? "Working" : "New"} />
          </Pressable>
          <Pressable
            disabled={pendingActions.sessions}
            onPress={onRefreshSessions}
            style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.sessions && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingActions.sessions} text={pendingActions.sessions ? "Loading" : "Refresh"} />
          </Pressable>
        </View>
      </View>

      <View style={styles.controlBlock}>
        <View style={styles.controlHeader}>
          <View>
            <Text style={styles.controlTitle}>Permission Mode</Text>
            <Text style={styles.settingMeta}>{permissionHelp(activePermission)}</Text>
          </View>
          {settingsBusy ? <ButtonContent loading text="Saving" /> : null}
        </View>
        <View style={styles.segmentRow}>
          {permissionModes.map((item) => {
            const selected = activePermission === item.mode;
            return (
              <Pressable
                disabled={!canUseSessionSettings || settingsBusy || selected}
                key={item.mode}
                onPress={() => onSetPermissionMode(item.mode)}
                style={({ pressed }) =>
                  buttonFeedback([styles.segment, selected && styles.segmentActive, (!canUseSessionSettings || settingsBusy) && styles.disabledButton], pressed)
                }
              >
                <Text style={styles.segmentTitle}>{item.label}</Text>
                <Text style={styles.segmentMeta}>{item.meta}</Text>
              </Pressable>
            );
          })}
        </View>
      </View>

      <View style={styles.controlBlock}>
        <View style={styles.controlHeader}>
          <View>
            <Text style={styles.controlTitle}>Context Window</Text>
            <Text style={styles.settingMeta}>{currentWindowK}K active window</Text>
          </View>
          <Pressable
            disabled={!canUseSessionSettings || settingsBusy}
            onPress={onCompactSession}
            style={({ pressed }) => buttonFeedback([styles.settingAction, (!canUseSessionSettings || settingsBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={settingsBusy} text={settingsBusy ? "Working" : "Compact"} />
          </Pressable>
        </View>
        <View style={styles.row}>
          <TextInput
            keyboardType="number-pad"
            onChangeText={setWindowInput}
            placeholder="16"
            placeholderTextColor="#776f66"
            style={[styles.input, styles.flex]}
            value={windowInput}
          />
          <Pressable
            disabled={!canUseSessionSettings || settingsBusy}
            onPress={submitWindow}
            style={({ pressed }) => buttonFeedback([styles.secondaryButton, (!canUseSessionSettings || settingsBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={settingsBusy} text="Apply" />
          </Pressable>
        </View>
        <View style={styles.segmentRow}>
          {contextPresets.map((preset) => (
            <Pressable
              disabled={!canUseSessionSettings || settingsBusy}
              key={preset}
              onPress={() => onSetContextWindowK(preset)}
              style={({ pressed }) =>
                buttonFeedback([styles.presetChip, currentWindowK === preset && styles.segmentActive, (!canUseSessionSettings || settingsBusy) && styles.disabledButton], pressed)
              }
            >
              <Text style={styles.segmentTitle}>{preset}K</Text>
            </Pressable>
          ))}
        </View>
        <View style={styles.contextGrid}>
          <ContextStat label="Full" value={context?.full_tokens} suffix="tok" />
          <ContextStat label="Selected" value={context?.selected_tokens} suffix="tok" />
          <ContextStat label="Summary" value={context?.summary_tokens} suffix="tok" />
          <ContextStat label="Messages" value={context?.selected_messages} suffix="shown" />
        </View>
        <Text style={styles.settingMeta}>
          {context?.has_summary ? "Summary exists" : "No summary yet"}
          {context?.truncated ? " / truncated" : ""}
        </Text>
      </View>

      {sessionID ? (
        <View style={styles.currentSessionBox}>
          <Text style={styles.currentSessionText}>{shortID(sessionID)}</Text>
          <Text style={styles.currentSessionMeta}>
            {activeSession?.model || "model"} / {activePermission} / {activeSession?.context_window_k || currentWindowK}K
            {activeSession?.usage?.total_tokens !== undefined ? ` / ${activeSession.usage.total_tokens} tokens` : ""}
          </Text>
        </View>
      ) : null}

      <ScrollView horizontal showsHorizontalScrollIndicator={false}>
        <View style={styles.sessionRow}>
          {sessions.length === 0 ? (
            <Text style={styles.emptyText}>{clientToken ? "No sessions loaded" : "Pair first"}</Text>
          ) : (
            sessions.map((session) => (
              <Pressable
                key={session.id}
                disabled={pendingActions.sessions}
                onPress={() => onLoadSession(session.id)}
                style={({ pressed }) =>
                  buttonFeedback([styles.sessionChip, session.id === sessionID && styles.sessionChipActive, pendingActions.sessions && styles.disabledButton], pressed)
                }
              >
                <Text style={styles.sessionTitle}>{session.title || "New chat"}</Text>
                <Text style={styles.sessionMeta}>
                  {shortID(session.id)} / {session.permission_mode || "ask"} / {session.context_window_k || 16}K
                </Text>
              </Pressable>
            ))
          )}
        </View>
      </ScrollView>
    </View>
  );
}

function IconBox({ label }: { label: string }) {
  return (
    <View style={styles.settingIconBox}>
      <Text style={styles.settingIconText}>{label}</Text>
    </View>
  );
}

function ContextStat({ label, suffix, value }: { label: string; suffix: string; value?: number }) {
  return (
    <View style={styles.contextStat}>
      <Text style={styles.contextValue}>{value ?? "-"}</Text>
      <Text style={styles.contextLabel}>{label} {suffix}</Text>
    </View>
  );
}

function normalizePermissionMode(mode?: string): SessionPermissionMode {
  if (mode === "readonly" || mode === "full") {
    return mode;
  }
  return "ask";
}

function permissionHelp(mode: SessionPermissionMode) {
  if (mode === "readonly") {
    return "Tools that write files or run commands are hidden.";
  }
  if (mode === "full") {
    return "Allowed tools can run without per-tool confirmation.";
  }
  return "Ask before write, shell, or sensitive tool calls.";
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 10,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  settingsPanel: {
    gap: 12,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  settingsTitle: {
    color: "#12100e",
    fontSize: 28,
    fontWeight: "900",
    lineHeight: 32,
  },
  pathText: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "700",
    marginTop: 3,
  },
  settingCard: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 0,
    borderWidth: 3,
    flexDirection: "row",
    gap: 12,
    minHeight: 74,
    padding: 12,
  },
  settingIconBox: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 0,
    borderWidth: 3,
    height: 48,
    justifyContent: "center",
    width: 56,
  },
  settingIconText: {
    color: "#12100e",
    fontSize: 18,
    fontWeight: "900",
  },
  settingTitle: {
    color: "#12100e",
    fontSize: 18,
    fontWeight: "900",
  },
  settingMeta: {
    color: "#6c665f",
    flexShrink: 1,
    fontSize: 12,
    fontWeight: "800",
    lineHeight: 16,
    marginTop: 4,
  },
  settingAction: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 0,
    borderWidth: 3,
    justifyContent: "center",
    minHeight: 44,
    minWidth: 58,
    paddingHorizontal: 10,
  },
  settingActionOnline: {
    backgroundColor: "#ff7f68",
  },
  settingActionWarn: {
    backgroundColor: "#ffd84f",
  },
  input: {
    backgroundColor: "#fdf7ea",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    color: "#12100e",
    includeFontPadding: false,
    lineHeight: 22,
    minHeight: 48,
    paddingHorizontal: 12,
    paddingVertical: 10,
    textAlignVertical: "center",
  },
  row: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  rowCompact: {
    flexDirection: "row",
    gap: 8,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  secondaryButton: {
    alignItems: "center",
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    justifyContent: "center",
    minHeight: 44,
    paddingHorizontal: 16,
  },
  smallButton: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  smallButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  disabledButton: {
    opacity: 0.45,
  },
  modelRow: {
    flexDirection: "row",
    gap: 8,
    paddingVertical: 2,
  },
  modelChip: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    minWidth: 132,
    padding: 10,
  },
  modelChipActive: {
    backgroundColor: "#b9e9b0",
  },
  modelTitle: {
    color: "#12100e",
    fontWeight: "900",
  },
  modelMeta: {
    color: "#6c665f",
    fontSize: 12,
    marginTop: 3,
  },
  controlBlock: {
    backgroundColor: "#fdf7ea",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 10,
    padding: 10,
  },
  controlHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  controlTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  segmentRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  segment: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexGrow: 1,
    minWidth: 92,
    padding: 9,
  },
  presetChip: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    minWidth: 58,
    paddingHorizontal: 10,
    paddingVertical: 9,
  },
  segmentActive: {
    backgroundColor: "#ffd84f",
  },
  segmentTitle: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  segmentMeta: {
    color: "#6c665f",
    fontSize: 11,
    marginTop: 3,
  },
  contextGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  contextStat: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    minWidth: 104,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  contextValue: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  contextLabel: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
    marginTop: 2,
  },
  currentSessionBox: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 3,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  currentSessionText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  currentSessionMeta: {
    color: "#6c665f",
    fontSize: 12,
  },
  sessionRow: {
    flexDirection: "row",
    gap: 8,
    paddingVertical: 2,
  },
  sessionChip: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    minWidth: 146,
    padding: 10,
  },
  sessionChipActive: {
    backgroundColor: "#ffd84f",
  },
  sessionTitle: {
    color: "#12100e",
    fontWeight: "900",
  },
  sessionMeta: {
    color: "#6c665f",
    fontSize: 12,
    marginTop: 3,
  },
  emptyText: {
    color: "#6c665f",
  },
});
