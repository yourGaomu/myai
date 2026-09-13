import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { Alert, Pressable, ScrollView, StyleSheet, Text, TextInput, useWindowDimensions, View } from "react-native";
import * as Updates from "expo-updates";

import { ButtonContent } from "../common/ButtonContent";
import { SubagentPanel } from "../subagents/SubagentPanel";
import { PluginPanel } from "../plugins/PluginPanel";
import { mobileAndroidVersionCode, mobileAppVersion } from "../../constants/app";
import type { CompactInfo, ContextInfo, GenerationSettings, ModelSummary, PluginInfo, SessionGenerationPreferences, SessionSummary, SkillSummary, SubagentDefinition, SubagentTask, SubagentTaskEvent } from "../../protocol";
import type { PendingAction, SessionAgentMode, SessionPermissionMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import type { ModelConfigDraft } from "../../hooks/useSessionModelActions";
import { shortID } from "../../utils/ids";
import { websocketURL } from "../../utils/relay";
import { modelDisplayName } from "../../utils/session";

type Props = {
  activeModel?: ModelSummary;
  activeSession?: SessionSummary;
  assetBaseURL: string;
  bindCode: string;
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  compact?: CompactInfo;
  connected: boolean;
  context?: ContextInfo;
  generation?: SessionGenerationPreferences;
  generationStatus?: { error: boolean; message: string };
  currentModelID: string;
  modelMessage: string;
  modelMessageError: boolean;
  deviceID: string;
  models: ModelSummary[];
  normalizedRelayURL: string;
  onBindCodeChange: (value: string) => void;
  onClose: () => void;
  onCompactSession: () => void;
  onConnect: () => void;
  onDeleteSession: (sessionID: string) => void;
  onDeviceIDChange: (value: string) => void;
  onExecutePlan: () => void;
  onLoadSession: (sessionID: string) => void;
  onNewSession: () => void;
  onOpenPlan: () => void;
  onPair: () => void;
  onRefreshModels: () => void;
  onAddModelConfig: (config: ModelConfigDraft) => void;
  onUpdateModelConfig: (config: ModelConfigDraft) => void;
  onDeleteModelConfig: (modelID: string) => void;
  onSetModelEnabled: (modelID: string, enabled: boolean) => void;
  onSetDefaultModel: (modelID: string) => void;
  onClearModelMessage: () => void;
  onTestModelConfig: (config: ModelConfigDraft) => void;
  onRequestContextInfo: () => void;
  onRequestGenerationPreferences: () => void;
  onRefreshSessions: () => void;
  onRefreshSkills: () => void;
  onReloadSkills: () => void;
  onRefreshPlugins: () => void;
  onReloadPlugins: () => void;
  onSetPluginEnabled: (pluginID: string, enabled: boolean) => void;
  onAssetBaseURLChange: (value: string) => void;
  onRelayURLChange: (value: string) => void;
  onSetAgentMode: (mode: SessionAgentMode) => void;
  onSetContextWindowK: (windowK: number) => void;
  onSetGenerationSettings: (settings: GenerationSettings) => void;
  onSetPermissionMode: (mode: SessionPermissionMode) => void;
  onSetStyleInstruction: (instruction: string) => void;
  onSwitchModel: (modelID: string) => void;
  onApplySubagentTask: (taskID: string) => void;
  onCancelSubagentTask: (taskID: string) => void;
  onCheckSubagentTask: (taskID: string) => void;
  onWaitSubagentTask: (taskID: string) => void;
  onCreateSubagentDefinition: (definition: Omit<SubagentDefinition, "id"> & { id?: string }) => boolean;
  onDeleteSubagentDefinition: (definitionID: string) => void;
  onDiscardSubagentTask: (taskID: string) => void;
  onFollowupSubagentTask: (taskID: string, message: string) => boolean;
  onMessageSubagentTask: (taskID: string, message: string) => boolean;
  onResumeSubagentTask: (taskID: string) => void;
  onRefreshSubagents: () => void;
  onUpdateSubagentDefinition: (definition: SubagentDefinition) => boolean;
  onUserIDChange: (value: string) => void;
  pendingActions: Record<PendingAction, boolean>;
  plugins: PluginInfo[];
  pluginMessage: string;
  pluginRoot: string;
  relayURL: string;
  sessionID: string;
  sessions: SessionSummary[];
  skillMessage: string;
  skillRoot: string;
  skills: SkillSummary[];
  subagentDefinitions: SubagentDefinition[];
  subagentEvents: Record<string, SubagentTaskEvent[]>;
  subagentMessage: string;
  subagentTasks: SubagentTask[];
  userID: string;
};

const permissionModes: Array<{ label: string; mode: SessionPermissionMode; meta: string }> = [
  { label: "只读", mode: "readonly", meta: "不可写入或运行" },
  { label: "询问", mode: "ask", meta: "工具调用需确认" },
  { label: "完全开放", mode: "full", meta: "自动允许工具调用" },
];
const agentModes: Array<{ label: string; mode: SessionAgentMode; meta: string }> = [
  { label: "对话", mode: "chat", meta: "正常对话与执行" },
  { label: "计划", mode: "plan", meta: "只读规划" },
];
const contextPresets = [8, 16, 32, 64, 128];
type SettingsSection = "general" | "connection" | "model" | "skill" | "plugin" | "subagent" | "session" | "permission" | "context" | "generation";
type ModelProtocol = "openai-chat-completions" | "anthropic-messages" | "google-generative-ai" | "mistral-chat" | "ollama-chat";

const modelProtocols: Array<{ label: string; meta: string; protocol: ModelProtocol }> = [
  { label: "OpenAI 兼容", meta: "聊天补全协议", protocol: "openai-chat-completions" },
  { label: "Anthropic", meta: "消息协议", protocol: "anthropic-messages" },
  { label: "Google Gemini", meta: "生成式 AI", protocol: "google-generative-ai" },
  { label: "Mistral", meta: "原生对话协议", protocol: "mistral-chat" },
  { label: "Ollama", meta: "本地原生对话", protocol: "ollama-chat" },
];

const settingSections: Array<{ icon: string; key: SettingsSection; label: string; meta: string }> = [
  { icon: "AG", key: "subagent", label: "子智能体", meta: "配置与后台任务" },
  { icon: "G", key: "general", label: "常规", meta: "状态总览" },
  { icon: "WS", key: "connection", label: "连接", meta: "中继服务与配对" },
  { icon: "AI", key: "model", label: "模型", meta: "选择当前模型" },
  { icon: "T", key: "generation", label: "生成", meta: "采样与回复风格" },
  { icon: "SK", key: "skill", label: "技能", meta: "本地技能中心" },
  { icon: "PL", key: "plugin", label: "插件", meta: "本地工具插件" },
  { icon: "S", key: "session", label: "会话", meta: "新建与切换" },
  { icon: "P", key: "permission", label: "权限", meta: "工具调用策略" },
  { icon: "K", key: "context", label: "上下文", meta: "窗口与压缩" },
];

export function SettingsPanel({
  activeModel,
  activeSession,
  assetBaseURL,
  bindCode,
  buttonFeedback,
  clientToken,
  compact,
  connected,
  context,
  generation,
  generationStatus,
  currentModelID,
  modelMessage,
  modelMessageError,
  deviceID,
  models,
  normalizedRelayURL,
  onBindCodeChange,
  onClose,
  onCompactSession,
  onConnect,
  onDeleteSession,
  onDeviceIDChange,
  onExecutePlan,
  onLoadSession,
  onNewSession,
  onOpenPlan,
  onPair,
  onRefreshModels,
  onAddModelConfig,
  onUpdateModelConfig,
  onDeleteModelConfig,
  onSetModelEnabled,
  onSetDefaultModel,
  onClearModelMessage,
  onTestModelConfig,
  onRequestContextInfo,
  onRequestGenerationPreferences,
  onRefreshSessions,
  onRefreshSkills,
  onReloadSkills,
  onRefreshPlugins,
  onReloadPlugins,
  onSetPluginEnabled,
  onAssetBaseURLChange,
  onRelayURLChange,
  onSetAgentMode,
  onSetContextWindowK,
  onSetGenerationSettings,
  onSetPermissionMode,
  onSetStyleInstruction,
  onSwitchModel,
  onApplySubagentTask,
  onCancelSubagentTask,
  onCheckSubagentTask,
  onWaitSubagentTask,
  onCreateSubagentDefinition,
  onDeleteSubagentDefinition,
  onDiscardSubagentTask,
  onFollowupSubagentTask,
  onMessageSubagentTask,
  onResumeSubagentTask,
  onRefreshSubagents,
  onUpdateSubagentDefinition,
  onUserIDChange,
  pendingActions,
  plugins,
  pluginMessage,
  pluginRoot,
  relayURL,
  sessionID,
  sessions,
  skillMessage,
  skillRoot,
  skills,
  subagentDefinitions,
  subagentEvents,
  subagentMessage,
  subagentTasks,
  userID,
}: Props) {
  const { width } = useWindowDimensions();
  const activePermission = normalizePermissionMode(activeSession?.permission_mode);
  const activeAgentMode = normalizeAgentMode(activeSession?.agent_mode);
  const activePlan = activeSession?.current_plan;
  const currentWindowK = context?.window_k || activeSession?.context_window_k || 16;
  const [windowInput, setWindowInput] = useState(String(currentWindowK));
  const [temperatureInput, setTemperatureInput] = useState("");
  const [topPInput, setTopPInput] = useState("");
  const [maxTokensInput, setMaxTokensInput] = useState("");
  const [styleInput, setStyleInput] = useState("");
  const [generationError, setGenerationError] = useState("");
  const [modelFormOpen, setModelFormOpen] = useState(false);
  const [editingModelID, setEditingModelID] = useState("");
  const [modelFormSubmitting, setModelFormSubmitting] = useState(false);
  const [modelFormError, setModelFormError] = useState("");
  const [modelForm, setModelForm] = useState({
    id: "",
    name: "",
    provider: "custom",
    protocol: "openai-chat-completions" as ModelProtocol,
    authType: "bearer",
    baseURL: "",
    apiKey: "",
    modelName: "",
    defaultTemperature: "",
    defaultTopP: "",
    defaultMaxTokens: "",
  });
  const [activeSection, setActiveSection] = useState<SettingsSection>("general");
  const requestContextInfoRef = useRef(onRequestContextInfo);
  const requestGenerationPreferencesRef = useRef(onRequestGenerationPreferences);

  useEffect(() => {
    setWindowInput(String(currentWindowK));
  }, [currentWindowK]);

  useEffect(() => {
    requestContextInfoRef.current = onRequestContextInfo;
  }, [onRequestContextInfo]);

  useEffect(() => {
    requestGenerationPreferencesRef.current = onRequestGenerationPreferences;
  }, [onRequestGenerationPreferences]);

  useEffect(() => {
    if (activeSection === "context" && connected && clientToken && sessionID) {
      requestContextInfoRef.current();
    }
  }, [activeSection, clientToken, connected, sessionID]);

  useEffect(() => {
    if (activeSection === "generation" && connected && clientToken && sessionID) {
      requestGenerationPreferencesRef.current();
    }
  }, [activeSection, activeSession?.model, clientToken, connected, sessionID]);

  useEffect(() => {
    setTemperatureInput(formatOverride(generation?.session_overrides.temperature));
    setTopPInput(formatOverride(generation?.session_overrides.top_p));
    setMaxTokensInput(formatOverride(generation?.session_overrides.max_output_tokens));
    setStyleInput(generation?.style_instruction || "");
    setGenerationError("");
  }, [generation]);

  useEffect(() => {
    if (!modelFormSubmitting || !modelMessage) {
      return;
    }
    if (modelMessageError) {
      setModelFormSubmitting(false);
      return;
    }
    setModelFormSubmitting(false);
    setModelFormOpen(false);
    resetModelForm();
  }, [modelFormSubmitting, modelMessage, modelMessageError]);

  const settingsBusy = pendingActions.settings;
  const generationBusy = pendingActions.generation;
  const contextBusy = pendingActions.context;
  const planBusy = pendingActions.plan;
  const canUseSessionSettings = Boolean(clientToken && sessionID);
  const wideLayout = width >= 760;
  const activeSectionMeta = settingSections.find((section) => section.key === activeSection);
  const versionLabel = `v${mobileAppVersion} · Android ${mobileAndroidVersionCode}`;
  const updateLabel = Updates.isEnabled
    ? `${Updates.channel || "本地"} · ${Updates.updateId ? shortID(Updates.updateId) : "嵌入包"}`
    : "开发模式 · OTA 未启用";
  const submitWindow = () => {
    const nextWindowK = Number.parseInt(windowInput, 10);
    if (Number.isNaN(nextWindowK)) {
      return;
    }
    onSetContextWindowK(nextWindowK);
  };

  const buildModelConfig = (allowEmptyAPIKey = false): ModelConfigDraft | null => {
    const id = modelForm.id.trim();
    const baseURL = modelForm.baseURL.trim();
    const apiKey = modelForm.apiKey.trim();
    const modelName = modelForm.modelName.trim() || id;
    const baseURLRequired = modelForm.protocol === "openai-chat-completions" || modelForm.protocol === "ollama-chat";
    if (!id || (baseURLRequired && !baseURL)) {
      setModelFormError(baseURLRequired ? "模型 ID 和 Base URL 不能为空。" : "模型 ID 不能为空。");
      return null;
    }
    if (modelForm.protocol === "google-generative-ai" && baseURL) {
      setModelFormError("Google Gemini 当前不支持自定义 Base URL，请留空。");
      return null;
    }
    if (modelForm.authType === "bearer" && !apiKey && !allowEmptyAPIKey) {
      setModelFormError("Bearer Token 认证必须填写 API Key。");
      return null;
    }

    const temperature = parseOptionalSetting(modelForm.defaultTemperature, "默认 Temperature", 0, 2);
    const topP = parseOptionalSetting(modelForm.defaultTopP, "默认 Top P", 0, 1);
    const maxTokens = parseOptionalInteger(modelForm.defaultMaxTokens, "默认最大输出 Token", 1, 131072);
    const error = temperature.error || topP.error || maxTokens.error;
    if (error) {
      setModelFormError(error);
      return null;
    }

    setModelFormError("");
    return {
      id,
      name: modelForm.name.trim() || id,
      provider: modelForm.provider.trim() || "custom",
      protocol: modelForm.protocol,
      auth_type: modelForm.authType,
      base_url: baseURL,
      api_key: apiKey,
      model_name: modelName,
      defaults: {
        temperature: temperature.value,
        top_p: topP.value,
        max_output_tokens: maxTokens.value,
      },
    };
  };

  const resetModelForm = () => {
    setEditingModelID("");
    setModelForm({
      id: "", name: "", provider: "custom", protocol: "openai-chat-completions", authType: "bearer", baseURL: "", apiKey: "", modelName: "",
      defaultTemperature: "", defaultTopP: "", defaultMaxTokens: "",
    });
  };

  const openModelEditor = (model: ModelSummary) => {
    setEditingModelID(model.id);
    setModelForm({
      id: model.id,
      name: model.name || model.id,
      provider: model.provider || "custom",
      protocol: (model.protocol as ModelProtocol) || "openai-chat-completions",
      authType: model.auth_type || "bearer",
      baseURL: model.base_url || "",
      apiKey: "",
      modelName: model.model_name || model.id,
      defaultTemperature: formatOverride(model.defaults?.temperature),
      defaultTopP: formatOverride(model.defaults?.top_p),
      defaultMaxTokens: formatOverride(model.defaults?.max_output_tokens),
    });
    setModelFormError("");
    onClearModelMessage();
    setModelFormOpen(true);
  };

  const confirmDeleteModel = (model: ModelSummary) => {
    const runDelete = () => onDeleteModelConfig(model.id);
    if (typeof window !== "undefined" && window.confirm) {
      if (window.confirm(`删除模型 ${modelDisplayName(model)}？`)) {
        runDelete();
      }
      return;
    }
    Alert.alert("删除模型？", modelDisplayName(model), [
      { text: "取消", style: "cancel" },
      { text: "删除", style: "destructive", onPress: runDelete },
    ]);
  };

  const relaySection = (
    <View style={styles.sectionStack}>
      <View style={[styles.settingCard, !wideLayout && styles.settingCardCompact]}>
        <IconBox label="WS" />
        <View style={[styles.flex, !wideLayout && styles.settingCardBody]}>
        <Text style={styles.settingTitle}>中继服务</Text>
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
        placeholder="https://relay.mikasa.wiki"
        placeholderTextColor="#776f66"
        style={styles.input}
        value={relayURL}
      />
      <TextInput
        autoCapitalize="none"
        autoCorrect={false}
        onChangeText={onAssetBaseURLChange}
        placeholder="资源服务地址，例如 https://assets.mikasa.wiki"
        placeholderTextColor="#776f66"
        style={styles.input}
        value={assetBaseURL}
      />
      <View style={styles.row}>
        <TextInput
          keyboardType="number-pad"
          maxLength={6}
          onChangeText={onBindCodeChange}
          placeholder="配对码"
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
          placeholder="用户标识"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.flex]}
          value={userID}
        />
        <TextInput
          onChangeText={onDeviceIDChange}
          placeholder="设备标识"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.flex]}
          value={deviceID}
        />
      </View>
    </View>
  );

  const modelSection = (
    <View style={styles.sectionStack}>
      <View style={[styles.settingCard, !wideLayout && styles.settingCardCompact]}>
        <IconBox label="AI" />
        <View style={[styles.flex, !wideLayout && styles.settingCardBody]}>
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
        <Pressable
          disabled={pendingActions.models}
          onPress={() => {
            const nextOpen = !modelFormOpen;
            setModelFormError("");
            setModelFormSubmitting(false);
            if (!nextOpen) {
              resetModelForm();
            }
            if (nextOpen) {
              onClearModelMessage();
            }
            setModelFormOpen(nextOpen);
          }}
          style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.models && styles.disabledButton], pressed)}
        >
          <ButtonContent text={modelFormOpen ? "取消添加" : "添加第三方模型"} />
        </Pressable>
      </View>

      {modelFormOpen ? (
        <View style={styles.controlBlock}>
          <Text style={styles.controlTitle}>{editingModelID ? "编辑模型" : "添加第三方模型"}</Text>
          <Text style={styles.settingMeta}>先选择模型协议，再填写该协议要求的连接信息。</Text>
          <View style={styles.modeRow}>
            {modelProtocols.map((item) => (
              <Pressable
                key={item.protocol}
                onPress={() => setModelForm((current) => ({
                  ...current,
                  protocol: item.protocol,
                  authType: item.protocol === "ollama-chat" ? "none" : "bearer",
                  apiKey: item.protocol === "ollama-chat" ? "" : current.apiKey,
                  baseURL: item.protocol === "google-generative-ai" ? "" : current.baseURL,
                }))}
                style={({ pressed }) => buttonFeedback([
                  styles.modeChip,
                  modelForm.protocol === item.protocol && styles.modeChipActive,
                ], pressed)}
              >
                <Text style={styles.modeChipTitle}>{item.label}</Text>
                <Text style={styles.modeChipMeta}>{item.meta}</Text>
              </Pressable>
            ))}
          </View>
          <Text style={styles.settingMeta}>{modelProtocolHelp(modelForm.protocol)}</Text>
          {([
            ["id", "模型 ID", "例如 deepseek-chat"],
            ["name", "显示名称", "可选"],
            ["provider", "服务商", "例如 deepseek、ollama"],
            ["baseURL", "服务地址", modelProtocolPlaceholder(modelForm.protocol)],
            ["apiKey", "API 密钥", "无认证服务可留空"],
            ["modelName", "厂商模型名", "例如 deepseek-chat"],
          ] as const).map(([key, label, placeholder]) => (
            <TextInput
              key={key}
              autoCapitalize="none"
              editable={key !== "baseURL" || modelForm.protocol !== "google-generative-ai"}
              onChangeText={(value) => setModelForm((current) => ({ ...current, [key]: value }))}
              placeholder={`${label}: ${placeholder}`}
              placeholderTextColor="#776f66"
              secureTextEntry={key === "apiKey"}
              style={[styles.input, key === "baseURL" && modelForm.protocol === "google-generative-ai" && styles.disabledInput]}
              value={modelForm[key]}
            />
          ))}
          <Text style={styles.settingMeta}>{editingModelID ? "API 密钥留空表示保留原密钥" : "认证方式"}</Text>
          <View style={styles.modeRow}>
            {([
              ["bearer", "Bearer 令牌", "需要 API 密钥"],
              ["none", "无认证", "本地服务常用"],
            ] as const).map(([value, label, meta]) => (
              <Pressable
                key={value}
                disabled={(modelForm.protocol === "ollama-chat" && value !== "none") ||
                  (value === "none" && modelForm.protocol !== "openai-chat-completions" && modelForm.protocol !== "ollama-chat")}
                onPress={() => setModelForm((current) => ({
                  ...current,
                  authType: value,
                  apiKey: value === "none" ? "" : current.apiKey,
                }))}
                style={({ pressed }) => buttonFeedback([
                  styles.modeChip,
                  modelForm.authType === value && styles.modeChipActive,
                  ((modelForm.protocol === "ollama-chat" && value !== "none") ||
                    (value === "none" && modelForm.protocol !== "openai-chat-completions" && modelForm.protocol !== "ollama-chat")) && styles.disabledButton,
                ], pressed)}
              >
                <Text style={styles.modeChipTitle}>{label}</Text>
                <Text style={styles.modeChipMeta}>{meta}</Text>
              </Pressable>
            ))}
          </View>
          <Text style={styles.settingMeta}>模型默认生成参数（留空表示使用系统默认值）</Text>
          <View style={[styles.row, styles.generationDefaultsRow]}>
            <TextInput
              keyboardType="decimal-pad"
              onChangeText={(value) => setModelForm((current) => ({ ...current, defaultTemperature: value }))}
              placeholder="温度 0-2"
              placeholderTextColor="#776f66"
              style={[styles.input, styles.flex, styles.generationDefaultsInput]}
              value={modelForm.defaultTemperature}
            />
            <TextInput
              keyboardType="decimal-pad"
              onChangeText={(value) => setModelForm((current) => ({ ...current, defaultTopP: value }))}
              placeholder="核采样 0-1"
              placeholderTextColor="#776f66"
              style={[styles.input, styles.flex, styles.generationDefaultsInput]}
              value={modelForm.defaultTopP}
            />
            <TextInput
              keyboardType="number-pad"
              onChangeText={(value) => setModelForm((current) => ({ ...current, defaultMaxTokens: value }))}
              placeholder="最大输出长度"
              placeholderTextColor="#776f66"
              style={[styles.input, styles.flex, styles.generationDefaultsInput]}
              value={modelForm.defaultMaxTokens}
            />
          </View>
          <View style={styles.row}>
            <Pressable
              disabled={pendingActions.models}
              onPress={() => {
                const config = buildModelConfig(Boolean(editingModelID));
                if (!config) {
                  return;
                }
                onClearModelMessage();
                setModelFormSubmitting(true);
                if (editingModelID) {
                  onUpdateModelConfig(config);
                } else {
                  onAddModelConfig(config);
                }
              }}
              style={({ pressed }) => buttonFeedback([styles.secondaryButton, pendingActions.models && styles.disabledButton], pressed)}
            >
              <ButtonContent loading={pendingActions.models} text={editingModelID ? "保存修改" : "保存模型"} />
            </Pressable>
            <Pressable
              disabled={pendingActions.models}
              onPress={() => {
                const config = buildModelConfig();
                if (!config) {
                  return;
                }
                onClearModelMessage();
                onTestModelConfig(config);
              }}
              style={({ pressed }) => buttonFeedback([styles.settingAction, pendingActions.models && styles.disabledButton], pressed)}
            >
              <ButtonContent loading={pendingActions.models} text="测试连接" />
            </Pressable>
          </View>
          {modelFormError ? <Text style={styles.generationMessage}>{modelFormError}</Text> : null}
        </View>
      ) : null}

      {models.length > 0 ? (
        <View style={styles.modelGrid}>
          {models.map((model) => {
            const selected = model.id === currentModelID;
            const disabled = model.enabled === false;
            return (
              <View key={model.id} style={[styles.modelChip, selected && styles.modelChipActive, disabled && styles.disabledButton]}>
                <Pressable
                  disabled={disabled || selected || pendingActions.models}
                  onPress={() => onSwitchModel(model.id)}
                  style={({ pressed }) => buttonFeedback([styles.modelInfoButton], pressed)}
                >
                  <Text style={styles.modelTitle}>{modelDisplayName(model)}{model.is_default ? " · 默认" : ""}</Text>
                  <Text style={styles.modelMeta}>{model.provider || "服务商"} / {model.model_name || model.id}</Text>
                  <Text numberOfLines={1} style={styles.modelMeta}>
                    {(model.protocol || "openai-chat-completions")} · {(model.auth_type || "bearer")} · {model.has_api_key ? "已配置密钥" : "无密钥"}
                  </Text>
                  <Text style={styles.modelMeta}>{disabled ? "已禁用" : selected ? "当前会话正在使用" : "点击切换会话模型"}</Text>
                  {model.base_url ? <Text numberOfLines={1} style={styles.modelMeta}>{model.base_url}</Text> : null}
                </Pressable>
                <View style={styles.modelActions}>
                  <Pressable disabled={pendingActions.models} onPress={() => onSetDefaultModel(model.id)} style={({ pressed }) => buttonFeedback([styles.modelAction, model.is_default && styles.modelActionActive, pendingActions.models && styles.disabledButton], pressed)}>
                    <Text style={styles.modelActionText}>{model.is_default ? "默认" : "设为默认"}</Text>
                  </Pressable>
                  <Pressable disabled={pendingActions.models} onPress={() => onSetModelEnabled(model.id, disabled)} style={({ pressed }) => buttonFeedback([styles.modelAction, pendingActions.models && styles.disabledButton], pressed)}>
                    <Text style={styles.modelActionText}>{disabled ? "启用" : "禁用"}</Text>
                  </Pressable>
                  <Pressable disabled={pendingActions.models} onPress={() => openModelEditor(model)} style={({ pressed }) => buttonFeedback([styles.modelAction, pendingActions.models && styles.disabledButton], pressed)}>
                    <Text style={styles.modelActionText}>编辑</Text>
                  </Pressable>
                  <Pressable disabled={pendingActions.models} onPress={() => confirmDeleteModel(model)} style={({ pressed }) => buttonFeedback([styles.modelActionDanger, pendingActions.models && styles.disabledButton], pressed)}>
                    <Text style={styles.modelActionText}>删除</Text>
                  </Pressable>
                </View>
              </View>
            );
          })}
        </View>
      ) : (
        <EmptyBox text={clientToken ? "还没有加载模型，点击刷新试试" : "先完成配对，再加载模型"} />
      )}
      {modelMessage ? <Text style={modelMessageError ? styles.generationMessage : styles.generationSuccess}>{modelMessage}</Text> : null}
    </View>
  );

  const submitGeneration = () => {
    const temperature = parseOptionalSetting(temperatureInput, "温度", 0, 2);
    const topP = parseOptionalSetting(topPInput, "核采样", 0, 1);
    const maxOutputTokens = parseOptionalInteger(maxTokensInput, "最大输出 Token", 1, 131072);
    const error = temperature.error || topP.error || maxOutputTokens.error;
    if (error) {
      setGenerationError(error);
      return;
    }
    setGenerationError("");
    onSetGenerationSettings({
      temperature: temperature.value,
      top_p: topP.value,
      max_output_tokens: maxOutputTokens.value,
    });
  };

  const resetGeneration = () => {
    setTemperatureInput("");
    setTopPInput("");
    setMaxTokensInput("");
    setGenerationError("");
    onSetGenerationSettings({ temperature: null, top_p: null, max_output_tokens: null });
  };

  const submitStyle = () => {
    if (Array.from(styleInput).length > 2000) {
      setGenerationError("回复风格不能超过 2000 个字符。");
      return;
    }
    setGenerationError("");
    onSetStyleInstruction(styleInput.trim());
  };

  const generationSection = (
    <View style={styles.sectionStack}>
      <View style={styles.controlBlock}>
        <View style={styles.controlHeader}>
          <View style={styles.flex}>
            <Text style={styles.controlTitle}>会话生成参数</Text>
            <Text style={styles.settingMeta}>空值表示继承模型默认值；最终生效值会包含系统兜底值。</Text>
          </View>
          {generationBusy ? <ButtonContent loading text="保存中" /> : null}
        </View>
        {generation ? (
          <>
            <GenerationField
              effective={generation.effective.temperature}
              label="温度"
              modelDefault={generation.model_defaults.temperature}
              onChangeText={setTemperatureInput}
              onReset={() => setTemperatureInput("")}
              systemDefault={0.7}
              value={temperatureInput}
            />
            <GenerationField
              effective={generation.effective.top_p}
              label="核采样"
              modelDefault={generation.model_defaults.top_p}
              onChangeText={setTopPInput}
              onReset={() => setTopPInput("")}
              systemDefault={1}
              value={topPInput}
            />
            <GenerationField
              effective={generation.effective.max_output_tokens}
              integer
              label="最大输出令牌"
              modelDefault={generation.model_defaults.max_output_tokens}
              onChangeText={setMaxTokensInput}
              onReset={() => setMaxTokensInput("")}
              systemDefault={2048}
              value={maxTokensInput}
            />
            <View style={styles.row}>
              <Pressable
                disabled={!canUseSessionSettings || generationBusy}
                onPress={submitGeneration}
                style={({ pressed }) => buttonFeedback([styles.secondaryButton, (!canUseSessionSettings || generationBusy) && styles.disabledButton], pressed)}
              >
                <ButtonContent loading={generationBusy} text="应用参数" />
              </Pressable>
              <Pressable
                disabled={!canUseSessionSettings || generationBusy}
                onPress={resetGeneration}
                style={({ pressed }) => buttonFeedback([styles.settingAction, (!canUseSessionSettings || generationBusy) && styles.disabledButton], pressed)}
              >
                <ButtonContent loading={generationBusy} text="全部继承" />
              </Pressable>
            </View>
          </>
        ) : (
          <View style={styles.generationLoading}>
            <ButtonContent loading={generationBusy} text={generationBusy ? "正在读取生成参数" : "连接后读取生成参数"} />
          </View>
        )}
      </View>

      <View style={styles.controlBlock}>
        <View style={styles.controlHeader}>
          <View style={styles.flex}>
            <Text style={styles.controlTitle}>回复风格</Text>
            <Text style={styles.settingMeta}>只影响表达方式，不覆盖模式、工具权限和安全规则。</Text>
          </View>
          <Text style={styles.generationCount}>{Array.from(styleInput).length}/2000</Text>
        </View>
        <TextInput
          multiline
          onChangeText={setStyleInput}
          placeholder="例如：使用简洁的中文，先给结论，再给必要细节。"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.multilineInput]}
          textAlignVertical="top"
          value={styleInput}
        />
        <View style={styles.row}>
          <Pressable
            disabled={!canUseSessionSettings || generationBusy}
            onPress={submitStyle}
            style={({ pressed }) => buttonFeedback([styles.secondaryButton, (!canUseSessionSettings || generationBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={generationBusy} text="保存风格" />
          </Pressable>
          <Pressable
            disabled={!canUseSessionSettings || generationBusy}
            onPress={() => { setStyleInput(""); onSetStyleInstruction(""); }}
            style={({ pressed }) => buttonFeedback([styles.settingAction, (!canUseSessionSettings || generationBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={generationBusy} text="清除风格" />
          </Pressable>
        </View>
      </View>

      {generationError || generationStatus?.message ? (
        <Text style={generationError || generationStatus?.error ? styles.generationMessage : styles.generationSuccess}>
          {generationError || generationStatus?.message}
        </Text>
      ) : null}
    </View>
  );

  const skillSection = (
    <View style={styles.sectionStack}>
      <View style={[styles.settingCard, !wideLayout && styles.settingCardCompact]}>
        <IconBox label="SK" />
        <View style={[styles.flex, !wideLayout && styles.settingCardBody]}>
          <Text style={styles.settingTitle}>技能</Text>
          <Text numberOfLines={2} style={styles.settingMeta}>
            {skills.length} 个技能已加载{skillRoot ? ` / ${skillRoot}` : ""}
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

  const pluginSection = (
    <PluginPanel
      buttonFeedback={buttonFeedback}
      message={pluginMessage}
      onRefresh={onRefreshPlugins}
      onReload={onReloadPlugins}
      onSetEnabled={onSetPluginEnabled}
      pending={pendingActions.plugins}
      plugins={plugins}
      root={pluginRoot}
    />
  );

  const sessionSection = (
    <View style={styles.sectionStack}>
      <View style={[styles.settingCard, !wideLayout && styles.settingCardCompact]}>
        <IconBox label="S" />
        <View style={[styles.flex, !wideLayout && styles.settingCardBody]}>
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
                {activeSession?.model || "model"} / {activeAgentMode} / {activePermission} / {activeSession?.context_window_k || currentWindowK}K
                {activeSession?.usage?.total_tokens !== undefined ? ` / ${activeSession.usage.total_tokens} 个令牌` : ""}
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
          <View style={styles.modeRow}>
            {agentModes.map((item) => {
              const selected = activeAgentMode === item.mode;
              return (
                <Pressable
                  disabled={!canUseSessionSettings || settingsBusy || selected}
                  key={item.mode}
                  onPress={() => onSetAgentMode(item.mode)}
                  style={({ pressed }) =>
                    buttonFeedback([styles.modeChip, selected && styles.modeChipActive, (!canUseSessionSettings || settingsBusy) && styles.disabledButton], pressed)
                  }
                >
                  <Text style={styles.modeChipTitle}>{item.label}</Text>
                  <Text style={styles.modeChipMeta}>{item.meta}</Text>
                </Pressable>
              );
            })}
          </View>
          {settingsBusy ? (
            <View style={styles.sessionBusyRow}>
              <ButtonContent loading text="正在切换会话模式" />
            </View>
          ) : null}
          {activePlan ? (
            <View style={styles.planSummaryBox}>
              <View style={styles.planSummaryHeader}>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.planSummaryTitle}>
                    计划 / {planStatusLabel(activePlan.status)} / {activePlan.steps?.length || 0} 个步骤
                  </Text>
                  <Text numberOfLines={2} style={styles.planSummaryMeta}>
                    {activePlan.goal || activePlan.steps?.[0]?.title || "结构化计划已准备好"}
                  </Text>
                </View>
                <View style={styles.planActionRow}>
                  <Pressable
                    disabled={!canUseSessionSettings}
                    onPress={onOpenPlan}
                    style={({ pressed }) => buttonFeedback([styles.planSmallButton, !canUseSessionSettings && styles.disabledButton], pressed)}
                  >
                    <Text style={styles.planSmallButtonText}>查看</Text>
                  </Pressable>
                  <Pressable
                    disabled={!canUseSessionSettings || planBusy || settingsBusy || activePlan.status === "done"}
                    onPress={onExecutePlan}
                    style={({ pressed }) =>
                      buttonFeedback(
                        [styles.planExecuteButton, (!canUseSessionSettings || planBusy || settingsBusy || activePlan.status === "done") && styles.disabledButton],
                        pressed,
                      )
                    }
                  >
                    <ButtonContent loading={planBusy} text={activePlan.status === "running" ? "执行中" : "执行"} />
                  </Pressable>
                </View>
              </View>
            </View>
          ) : null}
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
                <Text numberOfLines={1} style={styles.sessionTitle}>{session.title || "新对话"}</Text>
                <Text numberOfLines={1} style={styles.sessionMeta}>
                  {shortID(session.id)} / {session.agent_mode || "chat"} / {session.permission_mode || "ask"} / {session.context_window_k || 16}K
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
            <Text style={styles.settingMeta}>{currentWindowK}K 当前窗口</Text>
          </View>
          <Pressable
            disabled={!canUseSessionSettings || settingsBusy || contextBusy}
            onPress={onCompactSession}
            style={({ pressed }) => buttonFeedback([styles.settingAction, (!canUseSessionSettings || settingsBusy || contextBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={settingsBusy} text={settingsBusy ? "处理中" : "压缩"} />
          </Pressable>
        </View>
        {contextBusy ? (
          <View style={styles.contextLoadingRow}>
            <ButtonContent loading text="正在读取上下文" />
          </View>
        ) : null}
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
            disabled={!canUseSessionSettings || settingsBusy || contextBusy}
            onPress={submitWindow}
            style={({ pressed }) => buttonFeedback([styles.secondaryButton, (!canUseSessionSettings || settingsBusy || contextBusy) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={settingsBusy} text="应用" />
          </Pressable>
        </View>
        <View style={styles.segmentRow}>
          {contextPresets.map((preset) => (
            <Pressable
              disabled={!canUseSessionSettings || settingsBusy || contextBusy}
              key={preset}
              onPress={() => onSetContextWindowK(preset)}
              style={({ pressed }) =>
                buttonFeedback([styles.presetChip, currentWindowK === preset && styles.segmentActive, (!canUseSessionSettings || settingsBusy || contextBusy) && styles.disabledButton], pressed)
              }
            >
              <Text style={styles.segmentTitle}>{preset}K</Text>
            </Pressable>
          ))}
        </View>
        <View style={styles.contextGrid}>
          <ContextStat label="完整上下文" value={context?.full_tokens} suffix="令牌" />
          <ContextStat label="已选内容" value={context?.selected_tokens} suffix="令牌" />
          <ContextStat label="摘要" value={context?.summary_tokens} suffix="令牌" />
          <ContextStat label="前缀" value={context?.prefix_tokens} suffix="令牌" />
          <ContextStat label="可缓存" value={context?.cacheable_tokens} suffix="令牌" />
          <ContextStat label="消息数" value={context?.selected_messages} suffix="条" />
          <ContextStat label="版本" value={context?.summary_version} suffix="版" />
        </View>
        <View style={styles.hashGrid}>
          <HashPill label="前缀" value={context?.prefix_hash} />
          <HashPill label="摘要" value={context?.summary_hash} />
        </View>
        <View style={styles.contextSummaryBox}>
          <View style={styles.contextSummaryHeader}>
            <Text style={styles.compactTitle}>压缩摘要</Text>
            <Text style={styles.compactBadge}>{context?.has_summary ? `v${context.summary_version || 0}` : "未生成"}</Text>
          </View>
          {context?.summary?.trim() ? (
            <ScrollView nestedScrollEnabled style={styles.contextSummaryScroll}>
              <Text selectable style={styles.contextSummaryText}>{context.summary}</Text>
            </ScrollView>
          ) : (
            <Text style={styles.contextSummaryEmpty}>当前还没有压缩摘要。点击“压缩”后会在这里显示实际保存的摘要内容。</Text>
          )}
        </View>
        {context?.checkpoint ? <CheckpointDetails checkpoint={context.checkpoint} /> : null}
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
              <ContextStat label="压缩前" value={compact.before_tokens} suffix="令牌" />
              <ContextStat label="压缩后" value={compact.after_tokens} suffix="令牌" />
              <ContextStat label="新增" value={compact.new_messages} suffix="条消息" />
              <ContextStat label="可缓存" value={compact.cacheable_tokens} suffix="令牌" />
            </View>
            <View style={styles.hashGrid}>
              <HashPill label="前缀" value={compact.prefix_hash} />
              <HashPill label="摘要" value={compact.summary_hash} />
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
          <SummaryTile label="权限" value={permissionModeLabel(activePermission)} tone={activePermission === "full" ? "warn" : activePermission === "readonly" ? "quiet" : "normal"} />
        <SummaryTile label="上下文" value={`${currentWindowK}K`} />
        <SummaryTile label="设备" value={deviceID.trim() || "本地设备"} />
        <SummaryTile label="版本" value={versionLabel} tone="quiet" />
        <SummaryTile label="更新标识" value={updateLabel} />
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

  const subagentSection = (
    <SubagentPanel
      buttonFeedback={buttonFeedback}
      definitions={subagentDefinitions}
      message={subagentMessage}
      onApplyTask={onApplySubagentTask}
      onCancelTask={onCancelSubagentTask}
      onCheckTask={onCheckSubagentTask}
      onWaitTask={onWaitSubagentTask}
      onCreateDefinition={onCreateSubagentDefinition}
      onDeleteDefinition={onDeleteSubagentDefinition}
          onDiscardTask={onDiscardSubagentTask}
          onFollowupTask={onFollowupSubagentTask}
          onMessageTask={onMessageSubagentTask}
      onResumeTask={onResumeSubagentTask}
      onRefresh={onRefreshSubagents}
      onUpdateDefinition={onUpdateSubagentDefinition}
      pending={pendingActions.subagents}
      sessionID={sessionID}
      tasks={subagentTasks}
      events={subagentEvents}
    />
  );

  const sectionContent: Record<SettingsSection, ReactNode> = {
    connection: relaySection,
    context: contextSection,
    generation: generationSection,
    general: generalSection,
    model: modelSection,
    plugin: pluginSection,
    permission: permissionSection,
    session: sessionSection,
    skill: skillSection,
    subagent: subagentSection,
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

        <View style={[styles.contentPane, !wideLayout && styles.contentPaneCompact]}>
          <View style={styles.contentHeader}>
            <View style={styles.flex}>
              <Text style={styles.contentTitle}>{activeSectionMeta?.label || "设置"}</Text>
              <Text style={styles.settingMeta}>{activeSectionMeta?.meta || "配置中心"}</Text>
            </View>
          </View>
          {wideLayout ? (
            <ScrollView
              contentContainerStyle={styles.contentScroll}
              keyboardShouldPersistTaps="handled"
              nestedScrollEnabled
              showsVerticalScrollIndicator={false}
            >
              {sectionContent[activeSection]}
            </ScrollView>
          ) : (
            <View style={styles.contentScroll}>{sectionContent[activeSection]}</View>
          )}
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

function GenerationField({
  effective,
  integer = false,
  label,
  modelDefault,
  onChangeText,
  onReset,
  systemDefault,
  value,
}: {
  effective: number;
  integer?: boolean;
  label: string;
  modelDefault: number | null;
  onChangeText: (value: string) => void;
  onReset: () => void;
  systemDefault: number;
  value: string;
}) {
  const inheritedValue = modelDefault == null
    ? `模型未设置 · 系统兜底 ${formatNumber(systemDefault)}`
    : `模型默认 ${formatNumber(modelDefault)} · 系统兜底 ${formatNumber(systemDefault)}`;

  return (
    <View style={styles.generationField}>
      <View style={styles.generationFieldHeader}>
        <Text style={styles.generationLabel}>{label}</Text>
        <Text style={styles.generationEffective}>生效 {formatNumber(effective)}</Text>
      </View>
      <View style={styles.generationInputRow}>
        <TextInput
          keyboardType={integer ? "number-pad" : "decimal-pad"}
          onChangeText={onChangeText}
          placeholder="继承"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.generationInput]}
          value={value}
        />
        <Pressable onPress={onReset} style={styles.inheritButton}>
          <Text style={styles.inheritButtonText}>继承</Text>
        </Pressable>
      </View>
      <Text style={styles.generationMeta}>{inheritedValue} · 会话覆盖 {value.trim() || "未设置"}</Text>
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

function CheckpointDetails({ checkpoint }: { checkpoint: NonNullable<ContextInfo["checkpoint"]> }) {
  const [expanded, setExpanded] = useState(false);
  const data = checkpoint.summary_data;
  const sections: Array<[string, string[] | undefined]> = [
    ["用户偏好与约束", [...(data?.preferences || []), ...(data?.constraints || [])]],
    ["关键决策", data?.decisions],
    ["已完成工作", data?.completed_work],
    ["修改文件", data?.modified_files],
    ["工具与验证", data?.tool_verification],
    ["问题与根因", data?.problems],
    ["未完成任务", data?.open_tasks],
    ["下一步", data?.next_steps],
    ["重要引用", data?.references],
  ];
  return (
    <View style={styles.checkpointBox}>
      <Pressable onPress={() => setExpanded((value) => !value)} style={styles.checkpointHeader}>
        <View style={styles.flex}>
          <Text style={styles.compactTitle}>压缩检查点</Text>
          <Text style={styles.settingMeta}>版本 {checkpoint.version || 0} · 消息 {checkpoint.source_start_message} 至 {checkpoint.source_end_message}（结束索引不含）</Text>
        </View>
        <Text style={styles.checkpointToggle}>{expanded ? "收起" : "展开"}</Text>
      </Pressable>
      {expanded ? (
        <View style={styles.checkpointContent}>
          <HashPill label="源历史" value={checkpoint.source_history_hash} />
          {data?.current_goal ? <CheckpointSection title="当前目标" lines={[data.current_goal]} /> : null}
          {sections.map(([title, lines]) => <CheckpointSection key={title} title={title} lines={lines} />)}
        </View>
      ) : null}
    </View>
  );
}

function CheckpointSection({ title, lines }: { title: string; lines?: string[] }) {
  const visible = lines?.filter((line) => line.trim()) || [];
  if (visible.length === 0) {
    return null;
  }
  return (
    <View style={styles.checkpointSection}>
      <Text style={styles.checkpointSectionTitle}>{title}</Text>
      {visible.map((line, index) => <Text key={`${title}-${index}`} style={styles.checkpointLine}>- {line}</Text>)}
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

function normalizeAgentMode(mode?: string): SessionAgentMode {
  if (mode === "plan") {
    return "plan";
  }
  return "chat";
}

function permissionModeLabel(mode: SessionPermissionMode) {
  if (mode === "readonly") {
    return "只读";
  }
  if (mode === "full") {
    return "完全开放";
  }
  return "询问";
}

function planStatusLabel(status?: string) {
  switch (status) {
    case "draft": return "草稿";
    case "approved": return "已确认";
    case "running": return "执行中";
    case "done": return "已完成";
    case "failed": return "失败";
    case "canceled": return "已取消";
    default: return "空";
  }
}

function modelProtocolHelp(protocol: ModelProtocol) {
  switch (protocol) {
    case "openai-chat-completions":
      return "服务地址填写 API 根地址，例如 https://api.example.com/v1，不要填写 /chat/completions。";
    case "anthropic-messages":
      return "服务地址可留空使用 Anthropic 官方地址；自定义地址填写服务根地址。需要 API 密钥。";
    case "google-generative-ai":
      return "使用 Google 官方生成式 AI 地址，当前不支持自定义服务地址。需要 API 密钥。";
    case "mistral-chat":
      return "服务地址可留空使用 Mistral 官方地址；自定义地址填写服务根地址。需要 API 密钥。";
    case "ollama-chat":
      return "填写 Ollama 服务根地址，例如 http://127.0.0.1:11434，不要填写 /v1 或 /chat/completions。";
  }
}

function modelProtocolPlaceholder(protocol: ModelProtocol) {
  switch (protocol) {
    case "openai-chat-completions":
      return "例如 https://api.example.com/v1";
    case "anthropic-messages":
      return "可留空，或 https://api.anthropic.com";
    case "google-generative-ai":
      return "Google Gemini 不需要填写";
    case "mistral-chat":
      return "可留空，或 https://api.mistral.ai";
    case "ollama-chat":
      return "例如 http://127.0.0.1:11434";
  }
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

function formatOverride(value?: number | null) {
  return value === null || value === undefined ? "" : String(value);
}

function formatNumber(value: number) {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(4)));
}

function parseOptionalSetting(input: string, label: string, min: number, max: number): { value: number | null; error: string } {
  const normalized = input.trim();
  if (!normalized) {
    return { value: null, error: "" };
  }
  const value = Number(normalized);
  if (!Number.isFinite(value) || value < min || value > max) {
    return { value: null, error: `${label} 必须在 ${min} 到 ${max} 之间。` };
  }
  return { value, error: "" };
}

function parseOptionalInteger(input: string, label: string, min: number, max: number): { value: number | null; error: string } {
  const parsed = parseOptionalSetting(input, label, min, max);
  if (parsed.error || parsed.value === null) {
    return parsed;
  }
  if (!Number.isInteger(parsed.value)) {
    return { value: null, error: `${label} 必须是整数。` };
  }
  return parsed;
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: "#fffaf0",
    borderColor: "#d8cdbb",
    borderRadius: 16,
    borderWidth: 1,
    elevation: 1,
    gap: 10,
    padding: 12,
    shadowColor: "#8f8272",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.1,
    shadowRadius: 5,
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
    borderColor: "#ded4c6",
    borderRadius: 12,
    borderWidth: 1,
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
    borderRadius: 10,
    borderWidth: 1,
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
    borderColor: "#d8a900",
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
    borderColor: "#ded4c6",
    borderRadius: 14,
    borderWidth: 1,
    flex: 1,
    gap: 12,
    maxHeight: 680,
    minWidth: 0,
    padding: 12,
  },
  contentPaneCompact: {
    flex: 0,
    maxHeight: undefined,
    padding: 10,
    width: "100%",
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
    flexWrap: "wrap",
    gap: 12,
    minHeight: 74,
    padding: 12,
  },
  settingCardCompact: {
    alignItems: "flex-start",
  },
  settingCardBody: {
    minWidth: 112,
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
    flexShrink: 0,
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
  multilineInput: {
    minHeight: 116,
  },
  row: {
    alignItems: "center",
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  generationDefaultsRow: {
    alignItems: "stretch",
  },
  generationDefaultsInput: {
    minWidth: 120,
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
  disabledInput: {
    backgroundColor: "#ece7dd",
    opacity: 0.65,
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
  modelInfoButton: {
    flex: 1,
    minWidth: 0,
  },
  modelActions: {
    borderColor: "#12100e",
    borderTopWidth: 2,
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 5,
    marginTop: 8,
    paddingTop: 8,
  },
  modelAction: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 4,
    borderWidth: 2,
    paddingHorizontal: 7,
    paddingVertical: 5,
  },
  modelActionActive: {
    backgroundColor: "#ffd84f",
  },
  modelActionDanger: {
    backgroundColor: "#ffb4a7",
    borderColor: "#12100e",
    borderRadius: 4,
    borderWidth: 2,
    paddingHorizontal: 7,
    paddingVertical: 5,
  },
  modelActionText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
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
  generationField: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 6,
    padding: 9,
  },
  generationFieldHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  generationLabel: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
  },
  generationEffective: {
    color: "#1e6847",
    fontSize: 12,
    fontWeight: "900",
  },
  generationInputRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  generationInput: {
    flex: 1,
  },
  inheritButton: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 44,
    minWidth: 58,
    paddingHorizontal: 9,
  },
  inheritButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  generationMeta: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
  },
  generationCount: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "900",
  },
  generationLoading: {
    alignItems: "flex-start",
    minHeight: 48,
    justifyContent: "center",
  },
  generationMessage: {
    backgroundColor: "#ffe1d8",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    color: "#8c2d1c",
    fontSize: 13,
    fontWeight: "800",
    padding: 10,
  },
  generationSuccess: {
    backgroundColor: "#dff3dc",
    borderColor: "#1e6847",
    borderRadius: 8,
    borderWidth: 2,
    color: "#1e6847",
    fontSize: 13,
    fontWeight: "800",
    padding: 10,
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
  contextSummaryBox: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 8,
    padding: 10,
  },
  contextLoadingRow: {
    minHeight: 24,
  },
  contextSummaryHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  contextSummaryScroll: {
    maxHeight: 220,
  },
  contextSummaryText: {
    color: "#12100e",
    fontSize: 12,
    lineHeight: 19,
  },
  contextSummaryEmpty: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "800",
    lineHeight: 18,
  },
  checkpointBox: {
    backgroundColor: "#eef7ff",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 8,
    padding: 10,
  },
  checkpointHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  checkpointToggle: {
    color: "#1e5c9a",
    fontSize: 12,
    fontWeight: "900",
  },
  checkpointContent: {
    gap: 8,
  },
  checkpointSection: {
    gap: 3,
  },
  checkpointSectionTitle: {
    color: "#1e5c9a",
    fontSize: 11,
    fontWeight: "900",
  },
  checkpointLine: {
    color: "#12100e",
    fontSize: 12,
    lineHeight: 18,
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
  modeRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 8,
  },
  modeChip: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    flexGrow: 1,
    minWidth: 120,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  modeChipActive: {
    backgroundColor: "#ffd84f",
  },
  modeChipTitle: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  modeChipMeta: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
    marginTop: 2,
  },
  planSummaryBox: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    marginTop: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  planSummaryHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  planSummaryTitle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  planSummaryMeta: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
    marginTop: 3,
  },
  planActionRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  planSmallButton: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    minWidth: 56,
    paddingHorizontal: 8,
    paddingVertical: 7,
  },
  planSmallButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
    textAlign: "center",
  },
  planExecuteButton: {
    minWidth: 72,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  sessionBusyRow: {
    marginTop: 8,
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
