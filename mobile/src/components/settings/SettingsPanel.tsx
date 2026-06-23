import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, useWindowDimensions, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { CompactInfo, ContextInfo, ModelSummary, SessionSummary, SkillSummary } from "../../protocol";
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
  compact?: CompactInfo;
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
  onDeleteSession: (sessionID: string) => void;
  onDeviceIDChange: (value: string) => void;
  onLoadSession: (sessionID: string) => void;
  onNewSession: () => void;
  onPair: () => void;
  onRefreshModels: () => void;
  onRefreshSessions: () => void;
  onRefreshSkills: () => void;
  onReloadSkills: () => void;
  onRelayURLChange: (value: string) => void;
  onSetContextWindowK: (windowK: number) => void;
  onSetPermissionMode: (mode: SessionPermissionMode) => void;
  onSwitchModel: (modelID: string) => void;
  onUserIDChange: (value: string) => void;
  pendingActions: Record<PendingAction, boolean>;
  relayURL: string;
  sessionID: string;
  sessions: SessionSummary[];
  skillMessage: string;
  skillRoot: string;
  skills: SkillSummary[];
  userID: string;
};

const permissionModes: Array<{ label: string; mode: SessionPermissionMode; meta: string }> = [
  { label: "Read", mode: "readonly", meta: "no write/run" },
  { label: "Ask", mode: "ask", meta: "confirm tools" },
  { label: "Full", mode: "full", meta: "auto allow" },
];
const contextPresets = [8, 16, 32, 64, 128];
type SettingsSection = "general" | "connection" | "model" | "skill" | "session" | "permission" | "context";

const settingSections: Array<{ icon: string; key: SettingsSection; label: string; meta: string }> = [
  { icon: "G", key: "general", label: "常规", meta: "状态总览" },
  { icon: "WS", key: "connection", label: "连接", meta: "Relay 与配对" },
  { icon: "AI", key: "model", label: "模型", meta: "选择当前模型" },
  { icon: "SK", key: "skill", label: "技能", meta: "本地 SkillHub" },
  { icon: "S", key: "session", label: "会话", meta: "新建与切换" },
  { icon: "P", key: "permission", label: "权限", meta: "工具调用策略" },
  { icon: "K", key: "context", label: "上下文", meta: "窗口与压缩" },
];

export function SettingsPanel({
  activeModel,
  activeSession,
  bindCode,
  buttonFeedback,
  clientToken,
  compact,
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
  onDeleteSession,
  onDeviceIDChange,
  onLoadSession,
  onNewSession,
  onPair,
  onRefreshModels,
  onRefreshSessions,
  onRefreshSkills,
  onReloadSkills,
  onRelayURLChange,
  onSetContextWindowK,
  onSetPermissionMode,
  onSwitchModel,
  onUserIDChange,
  pendingActions,
  relayURL,
  sessionID,
  sessions,
  skillMessage,
  skillRoot,
  skills,
  userID,
}: Props) {
  const { width } = useWindowDimensions();
  const activePermission = normalizePermissionMode(activeSession?.permission_mode);
  const currentWindowK = context?.window_k || activeSession?.context_window_k || 16;
  const [windowInput, setWindowInput] = useState(String(currentWindowK));
  const [activeSection, setActiveSection] = useState<SettingsSection>("general");

  useEffect(() => {
    setWindowInput(String(currentWindowK));
  }, [currentWindowK]);

  const settingsBusy = pendingActions.settings;
  const canUseSessionSettings = Boolean(clientToken && sessionID);
  const wideLayout = width >= 760;
  const activeSectionMeta = settingSections.find((section) => section.key === activeSection);
  const submitWindow = () => {
    const nextWindowK = Number.parseInt(windowInput, 10);
    if (Number.isNaN(nextWindowK)) {
      return;
    }
    onSetContextWindowK(nextWindowK);
  };

  const relaySection = (
    <View style={styles.sectionStack}>
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
          <ButtonContent loading={pendingActions.connect} text={pendingActions.connect ? "连接中" : connected ? "在线" : "连接"} />
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
          <ButtonContent loading={pendingActions.pair} text={pendingActions.pair ? "配对中" : "配对"} />
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
    </View>
  );

  const modelSection = (
    <View style={styles.sectionStack}>
      <View style={styles.settingCard}>
        <IconBox label="AI" />
        <View style={styles.flex}>
          <Text style={styles.settingTitle}>模型</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>
            {activeModel ? modelDisplayName(activeModel) : currentModelID || activeSession?.model || "还没有加载模型"}
          </Text>
        </View>
        <Pressable
          disabled={pendingActions.models}
          onPress={onRefreshModels}
          style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.models && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.models} text={pendingActions.models ? "加载中" : "刷新"} />
        </Pressable>
      </View>

      {models.length > 0 ? (
        <View style={styles.modelGrid}>
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
      ) : (
        <EmptyBox text={clientToken ? "还没有加载模型，点击刷新试试" : "先完成配对，再加载模型"} />
      )}
    </View>
  );

  const skillSection = (
    <View style={styles.sectionStack}>
      <View style={styles.settingCard}>
        <IconBox label="SK" />
        <View style={styles.flex}>
          <Text style={styles.settingTitle}>技能</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>
            {skills.length} loaded{skillRoot ? ` / ${skillRoot}` : ""}
          </Text>
        </View>
        <View style={styles.rowCompact}>
          <Pressable
            disabled={pendingActions.skills}
            onPress={onRefreshSkills}
            style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.skills && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingActions.skills} text={pendingActions.skills ? "加载中" : "刷新"} />
          </Pressable>
          <Pressable
            disabled={pendingActions.skills}
            onPress={onReloadSkills}
            style={({ pressed }) => buttonFeedback([styles.secondaryButton, pendingActions.skills && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingActions.skills} text={pendingActions.skills ? "重载中" : "重载"} />
          </Pressable>
        </View>
      </View>

      {skillMessage ? <EmptyBox text={skillMessage} /> : null}

      {skills.length > 0 ? (
        <View style={styles.skillList}>
          {skills.map((item) => (
            <View key={`${item.name}:${item.path || ""}`} style={styles.skillCard}>
              <View style={styles.skillHeader}>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.skillName}>{item.name}</Text>
                  {item.description ? <Text numberOfLines={2} style={styles.skillDescription}>{item.description}</Text> : null}
                </View>
                <Text style={styles.skillBadge}>{formatDate(item.updated_at)}</Text>
              </View>
              {item.triggers && item.triggers.length > 0 ? (
                <View style={styles.triggerRow}>
                  {item.triggers.slice(0, 6).map((trigger) => (
                    <Text key={trigger} style={styles.triggerChip}>{trigger}</Text>
                  ))}
                </View>
              ) : null}
              {item.path ? <Text numberOfLines={1} style={styles.skillPath}>{item.path}</Text> : null}
            </View>
          ))}
        </View>
      ) : (
        <EmptyBox text={clientToken ? "还没有本地技能。可以用 SkillHub 安装，或创建 skills/<name>/SKILL.md。" : "先连接 agent，再查看本地技能。"} />
      )}
    </View>
  );

  const sessionSection = (
    <View style={styles.sectionStack}>
      <View style={styles.settingCard}>
        <IconBox label="S" />
        <View style={styles.flex}>
          <Text style={styles.settingTitle}>会话</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>
            {activeSession?.title || (sessionID ? shortID(sessionID) : "还没有选择会话")}
          </Text>
        </View>
        <View style={styles.rowCompact}>
          <Pressable
            disabled={pendingActions.sessions}
            onPress={onNewSession}
            style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.sessions && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingActions.sessions} text={pendingActions.sessions ? "处理中" : "新建"} />
          </Pressable>
          <Pressable
            disabled={pendingActions.sessions}
            onPress={onRefreshSessions}
            style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.sessions && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingActions.sessions} text={pendingActions.sessions ? "加载中" : "刷新"} />
          </Pressable>
        </View>
      </View>

      {sessionID ? (
        <View style={styles.currentSessionBox}>
          <View style={styles.currentSessionHeader}>
            <View style={styles.flex}>
              <Text style={styles.currentSessionText}>{shortID(sessionID)}</Text>
              <Text style={styles.currentSessionMeta}>
                {activeSession?.model || "model"} / {activePermission} / {activeSession?.context_window_k || currentWindowK}K
                {activeSession?.usage?.total_tokens !== undefined ? ` / ${activeSession.usage.total_tokens} tokens` : ""}
              </Text>
            </View>
            <Pressable
              disabled={pendingActions.sessions}
              onPress={() => onDeleteSession(sessionID)}
              style={({ pressed }) => buttonFeedback([styles.deleteButton, pendingActions.sessions && styles.disabledButton], pressed)}
            >
              <Text style={styles.deleteButtonText}>删除</Text>
            </Pressable>
          </View>
        </View>
      ) : null}

      <View style={styles.sessionGrid}>
        {sessions.length === 0 ? (
          <EmptyBox text={clientToken ? "还没有会话，点击新建开始" : "先配对后同步会话"} />
        ) : (
          sessions.map((session) => (
            <View
              key={session.id}
              style={[styles.sessionChip, session.id === sessionID && styles.sessionChipActive]}
            >
              <Pressable
                disabled={pendingActions.sessions}
                onPress={() => onLoadSession(session.id)}
                style={({ pressed }) => buttonFeedback([styles.sessionChipMain, pendingActions.sessions && styles.disabledButton], pressed)}
              >
                <Text numberOfLines={1} style={styles.sessionTitle}>{session.title || "New chat"}</Text>
                <Text numberOfLines={1} style={styles.sessionMeta}>
                  {shortID(session.id)} / {session.permission_mode || "ask"} / {session.context_window_k || 16}K
                </Text>
              </Pressable>
              <Pressable
                disabled={pendingActions.sessions}
                onPress={() => onDeleteSession(session.id)}
                style={({ pressed }) => buttonFeedback([styles.sessionDeleteButton, pendingActions.sessions && styles.disabledButton], pressed)}
              >
                <Text style={styles.deleteButtonText}>删除</Text>
              </Pressable>
            </View>
          ))
        )}
      </View>
    </View>
  );

  const permissionSection = (
    <View style={styles.sectionStack}>
      <View style={styles.controlBlock}>
        <View style={styles.controlHeader}>
          <View style={styles.flex}>
            <Text style={styles.controlTitle}>权限模式</Text>
            <Text style={styles.settingMeta}>{permissionHelp(activePermission)}</Text>
          </View>
          {settingsBusy ? <ButtonContent loading text="保存中" /> : null}
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
      <EmptyBox text="权限是会话级别配置。切换会话后，会使用对应会话自己的工具调用策略。" />
    </View>
  );

  const contextSection = (
    <View style={styles.sectionStack}>
      <View style={styles.controlBlock}>
        <View style={styles.controlHeader}>
          <View style={styles.flex}>
            <Text style={styles.controlTitle}>上下文窗口</Text>
            <Text style={styles.settingMeta}>{currentWindowK}K active window</Text>
          </View>
          <Pressable
            disabled={!canUseSessionSettings || settingsBusy}
            onPress={onCompactSession}
            style={({ pressed }) => buttonFeedback([styles.settingAction, (!canUseSessionSettings || settingsBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={settingsBusy} text={settingsBusy ? "处理中" : "压缩"} />
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
            <ButtonContent loading={settingsBusy} text="应用" />
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
          <ContextStat label="Prefix" value={context?.prefix_tokens} suffix="tok" />
          <ContextStat label="Cacheable" value={context?.cacheable_tokens} suffix="tok" />
          <ContextStat label="Messages" value={context?.selected_messages} suffix="shown" />
          <ContextStat label="Version" value={context?.summary_version} suffix="sum" />
        </View>
        <View style={styles.hashGrid}>
          <HashPill label="prefix" value={context?.prefix_hash} />
          <HashPill label="summary" value={context?.summary_hash} />
        </View>
        <View style={styles.contextNote}>
          <Text style={styles.contextNoteText}>
            {context?.has_summary ? "已有摘要，会把老消息压成稳定前缀" : "还没有摘要，当前主要发送最近消息"}
            {context?.truncated ? " / 已触发窗口裁剪" : ""}
          </Text>
        </View>
        {compact?.triggered ? (
          <View style={styles.compactBox}>
            <View style={styles.compactHeader}>
              <Text style={styles.compactTitle}>上次自动压缩</Text>
              <Text style={styles.compactBadge}>{compactReasonLabel(compact.reason)}</Text>
            </View>
            <View style={styles.contextGrid}>
              <ContextStat label="Before" value={compact.before_tokens} suffix="tok" />
              <ContextStat label="After" value={compact.after_tokens} suffix="tok" />
              <ContextStat label="New" value={compact.new_messages} suffix="msg" />
              <ContextStat label="Cacheable" value={compact.cacheable_tokens} suffix="tok" />
            </View>
            <View style={styles.hashGrid}>
              <HashPill label="prefix" value={compact.prefix_hash} />
              <HashPill label="summary" value={compact.summary_hash} />
            </View>
          </View>
        ) : (
          <Text style={styles.settingMeta}>自动压缩会在上下文接近窗口 70% 或必须裁剪时触发。</Text>
        )}
      </View>
    </View>
  );

  const generalSection = (
    <View style={styles.sectionStack}>
      <View style={styles.summaryGrid}>
        <SummaryTile label="连接" value={connected ? "在线" : clientToken ? "未连接" : "未配对"} tone={connected ? "good" : "warn"} />
        <SummaryTile label="模型" value={activeModel ? modelDisplayName(activeModel) : currentModelID || "未加载"} />
        <SummaryTile label="技能" value={`${skills.length}`} tone={skills.length > 0 ? "good" : "quiet"} />
        <SummaryTile label="会话" value={activeSession?.title || (sessionID ? shortID(sessionID) : "未选择")} />
        <SummaryTile label="权限" value={activePermission} tone={activePermission === "full" ? "warn" : activePermission === "readonly" ? "quiet" : "normal"} />
        <SummaryTile label="上下文" value={`${currentWindowK}K`} />
        <SummaryTile label="设备" value={deviceID.trim() || "pc-local"} />
      </View>
      <View style={styles.quickActions}>
        <Pressable
          disabled={pendingActions.connect}
          onPress={onConnect}
          style={({ pressed }) => buttonFeedback([styles.secondaryButton, pendingActions.connect && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.connect} text={connected ? "重新连接" : "连接"} />
        </Pressable>
        <Pressable
          disabled={pendingActions.models}
          onPress={onRefreshModels}
          style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.models && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.models} text="刷新模型" />
        </Pressable>
        <Pressable
          disabled={pendingActions.skills}
          onPress={onRefreshSkills}
          style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.skills && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.skills} text="刷新技能" />
        </Pressable>
        <Pressable
          disabled={pendingActions.sessions}
          onPress={onRefreshSessions}
          style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.sessions && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingActions.sessions} text="刷新会话" />
        </Pressable>
      </View>
    </View>
  );

  const sectionContent: Record<SettingsSection, ReactNode> = {
    connection: relaySection,
    context: contextSection,
    general: generalSection,
    model: modelSection,
    permission: permissionSection,
    session: sessionSection,
    skill: skillSection,
  };

  return (
    <View style={[styles.panel, wideLayout && styles.panelWide]}>
      <View style={styles.panelHeader}>
        <View style={styles.flex}>
          <Text style={styles.settingsTitle}>设置</Text>
          <Text style={styles.pathText}>连接、模型、会话和工具权限集中管理</Text>
        </View>
        <Pressable onPress={onClose} style={({ pressed }) => buttonFeedback(styles.smallButton, pressed)}>
          <Text style={styles.smallButtonText}>完成</Text>
        </Pressable>
      </View>

      <View style={[styles.settingsBox, wideLayout && styles.settingsBoxWide]}>
        <View style={[styles.sideRail, wideLayout ? styles.sideRailWide : styles.sideRailCompact]}>
          <ScrollView
            horizontal={!wideLayout}
            showsHorizontalScrollIndicator={false}
            showsVerticalScrollIndicator={false}
          >
            <View style={[styles.navList, wideLayout ? styles.navListWide : styles.navListCompact]}>
              {settingSections.map((section) => {
                const selected = activeSection === section.key;
                return (
                  <Pressable
                    key={section.key}
                    onPress={() => setActiveSection(section.key)}
                    style={({ pressed }) =>
                      buttonFeedback([styles.navItem, wideLayout && styles.navItemWide, selected && styles.navItemActive], pressed)
                    }
                  >
                    <Text style={[styles.navIcon, selected && styles.navTextActive]}>{section.icon}</Text>
                    <View style={styles.navTextBlock}>
                      <Text style={[styles.navLabel, selected && styles.navTextActive]}>{section.label}</Text>
                      {wideLayout ? <Text style={styles.navMeta}>{section.meta}</Text> : null}
                    </View>
                  </Pressable>
                );
              })}
            </View>
          </ScrollView>
        </View>

        <View style={styles.contentPane}>
          <View style={styles.contentHeader}>
            <View style={styles.flex}>
              <Text style={styles.contentTitle}>{activeSectionMeta?.label || "设置"}</Text>
              <Text style={styles.settingMeta}>{activeSectionMeta?.meta || "配置中心"}</Text>
            </View>
          </View>
          <ScrollView
            contentContainerStyle={styles.contentScroll}
            nestedScrollEnabled
            showsVerticalScrollIndicator={false}
          >
            {sectionContent[activeSection]}
          </ScrollView>
        </View>
      </View>
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

function HashPill({ label, value }: { label: string; value?: string }) {
  return (
    <View style={styles.hashPill}>
      <Text style={styles.hashLabel}>{label}</Text>
      <Text numberOfLines={1} style={styles.hashValue}>{shortHash(value)}</Text>
    </View>
  );
}

function EmptyBox({ text }: { text: string }) {
  return (
    <View style={styles.emptyBox}>
      <Text style={styles.emptyText}>{text}</Text>
    </View>
  );
}

function SummaryTile({
  label,
  tone = "normal",
  value,
}: {
  label: string;
  tone?: "good" | "normal" | "quiet" | "warn";
  value: string;
}) {
  return (
    <View style={[styles.summaryTile, tone === "good" && styles.summaryTileGood, tone === "warn" && styles.summaryTileWarn, tone === "quiet" && styles.summaryTileQuiet]}>
      <Text style={styles.summaryLabel}>{label}</Text>
      <Text numberOfLines={2} style={styles.summaryValue}>{value}</Text>
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
    return "禁止写文件和运行命令类工具。";
  }
  if (mode === "full") {
    return "允许的工具可以不再逐次确认。";
  }
  return "写文件、Shell、敏感工具调用前需要确认。";
}

function shortHash(value?: string) {
  if (!value) {
    return "-";
  }
  return value.length <= 12 ? value : `${value.slice(0, 6)}...${value.slice(-4)}`;
}

function compactReasonLabel(reason?: string) {
  if (reason === "window_limit") {
    return "窗口限制";
  }
  if (reason === "threshold") {
    return "阈值触发";
  }
  return reason || "自动";
}

function formatDate(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return `${date.getMonth() + 1}/${date.getDate()}`;
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
  panelWide: {
    alignSelf: "center",
    maxWidth: 1180,
    width: "100%",
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
  settingsBox: {
    gap: 12,
  },
  settingsBoxWide: {
    alignItems: "stretch",
    flexDirection: "row",
  },
  sideRail: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
  },
  sideRailWide: {
    flexShrink: 0,
    minHeight: 520,
    padding: 8,
    width: 222,
  },
  sideRailCompact: {
    padding: 6,
  },
  navList: {
    gap: 6,
  },
  navListWide: {
    width: "100%",
  },
  navListCompact: {
    flexDirection: "row",
  },
  navItem: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "transparent",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 8,
    minHeight: 48,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  navItemWide: {
    width: "100%",
  },
  navItemActive: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
  },
  navIcon: {
    color: "#12100e",
    fontSize: 15,
    fontWeight: "900",
    minWidth: 22,
    textAlign: "center",
  },
  navTextBlock: {
    minWidth: 0,
  },
  navLabel: {
    color: "#3b3834",
    fontSize: 14,
    fontWeight: "900",
  },
  navMeta: {
    color: "#7f766c",
    fontSize: 11,
    fontWeight: "700",
    marginTop: 2,
  },
  navTextActive: {
    color: "#12100e",
  },
  contentPane: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flex: 1,
    gap: 12,
    maxHeight: 680,
    minWidth: 0,
    padding: 12,
  },
  contentScroll: {
    paddingBottom: 4,
  },
  contentHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  contentTitle: {
    color: "#12100e",
    fontSize: 22,
    fontWeight: "900",
    lineHeight: 26,
  },
  sectionStack: {
    gap: 10,
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
    flexWrap: "wrap",
    gap: 8,
  },
  rowCompact: {
    flexDirection: "row",
    flexWrap: "wrap",
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
  modelGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    paddingVertical: 2,
  },
  modelChip: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexGrow: 1,
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
  skillList: {
    gap: 8,
  },
  skillCard: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 8,
    padding: 10,
  },
  skillHeader: {
    alignItems: "flex-start",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  skillName: {
    color: "#12100e",
    fontSize: 15,
    fontWeight: "900",
  },
  skillDescription: {
    color: "#4f4942",
    fontSize: 12,
    fontWeight: "800",
    lineHeight: 17,
    marginTop: 3,
  },
  skillBadge: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  triggerRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 6,
  },
  triggerChip: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  skillPath: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
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
    flexGrow: 1,
    minWidth: 104,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  hashGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  hashPill: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    flexDirection: "row",
    flexGrow: 1,
    gap: 8,
    minHeight: 36,
    minWidth: 146,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  hashLabel: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "900",
    textTransform: "uppercase",
  },
  hashValue: {
    color: "#12100e",
    flex: 1,
    fontSize: 12,
    fontWeight: "900",
    minWidth: 0,
  },
  contextNote: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  contextNoteText: {
    color: "#4f4942",
    fontSize: 12,
    fontWeight: "800",
    lineHeight: 17,
  },
  compactBox: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 8,
    padding: 10,
  },
  compactHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  compactTitle: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
  },
  compactBadge: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  summaryGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  summaryTile: {
    backgroundColor: "#fdf7ea",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    flexBasis: 150,
    flexGrow: 1,
    minHeight: 72,
    padding: 10,
  },
  summaryTileGood: {
    backgroundColor: "#b9e9b0",
  },
  summaryTileWarn: {
    backgroundColor: "#ffd84f",
  },
  summaryTileQuiet: {
    backgroundColor: "#f5eefc",
  },
  summaryLabel: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "900",
  },
  summaryValue: {
    color: "#12100e",
    fontSize: 15,
    fontWeight: "900",
    lineHeight: 19,
    marginTop: 5,
  },
  quickActions: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
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
  currentSessionHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
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
  sessionGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    paddingVertical: 2,
  },
  sessionChip: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexGrow: 1,
    minWidth: 146,
    padding: 10,
  },
  sessionChipMain: {
    minWidth: 0,
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
  deleteButton: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 10,
  },
  sessionDeleteButton: {
    alignSelf: "flex-start",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    marginTop: 8,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  deleteButtonText: {
    color: "#a3342f",
    fontSize: 12,
    fontWeight: "900",
  },
  emptyText: {
    color: "#6c665f",
    fontWeight: "800",
  },
  emptyBox: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    paddingHorizontal: 10,
    paddingVertical: 10,
  },
});
