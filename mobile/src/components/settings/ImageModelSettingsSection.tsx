import { useEffect, useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { ButtonContent } from "../common/ButtonContent";
import { ConfirmModal } from "../common/ConfirmModal";
import { ResponsiveFormModal } from "../common/ResponsiveFormModal";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import type {
  ImageModelConfigAddPayload,
  ImageModelConfigTestPayload,
  ImageModelConfigUpdatePayload,
  ImageModelSummary,
} from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";

const protocols = [
  { value: "openai-images", label: "OpenAI Images", hint: "OpenAI 兼容图片生成与参考图编辑" },
  { value: "gemini-native-image", label: "Gemini Native", hint: "Gemini 原生图片生成，支持参考图" },
  { value: "google-imagen", label: "Google Imagen", hint: "Vertex AI 文生图，不支持参考图" },
] as const;
type Protocol = typeof protocols[number]["value"];
type AuthType = ImageModelSummary["auth_type"];
type Draft = {
  id: string; name: string; provider: string; protocol: Protocol; auth_type: AuthType;
  base_url: string; api_key: string; model_name: string; is_default: boolean; enabled: boolean;
};
type Props = {
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  defaultModelID: string;
  loading: boolean;
  message: string;
  messageError: boolean;
  models: ImageModelSummary[];
  testResult: { ok: boolean; message: string; latency_ms: number; mime_type?: string } | null;
  onRefresh: () => boolean;
  onAdd: (payload: ImageModelConfigAddPayload) => boolean;
  onUpdate: (payload: ImageModelConfigUpdatePayload) => boolean;
  onTest: (payload: ImageModelConfigTestPayload) => boolean;
  onDelete: (id: string) => boolean;
  onSetEnabled: (id: string, enabled: boolean) => boolean;
  onSetDefault: (id: string) => boolean;
  onClearMessage: () => void;
};

function defaultAuthType(protocol: Protocol): AuthType {
  if (protocol === "gemini-native-image") return "x-goog-api-key";
  if (protocol === "google-imagen") return "google-adc";
  return "bearer";
}
function blankDraft(defaultModel = false): Draft {
  const protocol: Protocol = "openai-images";
  return { id: "", name: "", provider: "", protocol, auth_type: defaultAuthType(protocol), base_url: "", api_key: "", model_name: "", is_default: defaultModel, enabled: true };
}
function editDraft(model: ImageModelSummary): Draft {
  return { id: model.id, name: model.name, provider: model.provider, protocol: model.protocol, auth_type: model.auth_type, base_url: model.base_url, api_key: "", model_name: model.model_name, is_default: model.is_default, enabled: model.enabled };
}

export function ImageModelSettingsSection(props: Props) {
  const { buttonFeedback, connected, defaultModelID, loading, message, messageError, models, testResult } = props;
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const [formVisible, setFormVisible] = useState(false);
  const [draft, setDraft] = useState<Draft>(() => blankDraft(!defaultModelID));
  const [editingID, setEditingID] = useState("");
  const [formError, setFormError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ImageModelSummary | null>(null);
  const [formAction, setFormAction] = useState<"test" | "save" | null>(null);

  const onRefresh = props.onRefresh;
  useEffect(() => {
    if (connected) onRefresh();
  }, [connected, onRefresh]);

  const allowedAuth = useMemo(() => {
    if (draft.protocol === "gemini-native-image") return [
      ["x-goog-api-key", "Google API Key"], ["bearer", "Bearer 令牌"],
    ] as const;
    if (draft.protocol === "google-imagen") return [
      ["google-adc", "Google ADC（推荐）"], ["bearer", "Bearer 令牌"],
    ] as const;
    return [["bearer", "Bearer 令牌"], ["none", "无认证"]] as const;
  }, [draft.protocol]);

  const closeForm = () => {
    setFormVisible(false);
    setFormError("");
    setFormAction(null);
    setDraft(blankDraft(!defaultModelID));
    setEditingID("");
    props.onClearMessage();
  };
  const openAdd = () => {
    props.onClearMessage();
    setEditingID("");
    setDraft(blankDraft(!defaultModelID));
    setFormError("");
    setFormAction(null);
    setFormVisible(true);
  };
  const openEdit = (model: ImageModelSummary) => {
    props.onClearMessage();
    setEditingID(model.id);
    setDraft(editDraft(model));
    setFormError("");
    setFormAction(null);
    setFormVisible(true);
  };
  const updateDraft = (key: keyof Draft, value: string | boolean) => setDraft((current) => ({ ...current, [key]: value }));
  const validate = () => {
    if (!editingID && !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(draft.id.trim())) return "模型 ID 必须以字母或数字开头，只能包含字母、数字、点、下划线和连字符。";
    if (!draft.model_name.trim()) return "请填写服务商提供的模型 ID。";
    if (draft.protocol === "google-imagen" && !draft.base_url.trim()) return "Google Imagen 需要填写 Vertex AI 项目与区域地址。";
    if (draft.auth_type !== "none" && draft.auth_type !== "google-adc" && !draft.api_key.trim()) {
      const saved = editingID ? models.find((item) => item.id === editingID) : undefined;
      const canReuse = saved && saved.auth_type === draft.auth_type && saved.protocol === draft.protocol && saved.base_url === draft.base_url.trim();
      if (!canReuse) return "请填写 API Key；更换协议、认证方式或服务地址时需要重新提供密钥。";
    }
    return "";
  };
  const requestPayload = () => ({
    ...(editingID ? { id: editingID } : { id: draft.id.trim() }),
    name: draft.name.trim() || draft.id.trim() || editingID,
    provider: draft.provider.trim() || draft.protocol,
    protocol: draft.protocol,
    auth_type: draft.auth_type,
    base_url: draft.base_url.trim(),
    api_key: draft.api_key.trim(),
    model_name: draft.model_name.trim(),
  });
  const runTest = () => {
    const error = validate();
    if (error) { setFormError(error); return; }
    setFormError("");
    props.onClearMessage();
    setFormAction("test");
    const payload = requestPayload();
    const sent = props.onTest({
      ...(editingID ? { id: editingID } : {}),
      protocol: payload.protocol,
      auth_type: payload.auth_type,
      base_url: payload.base_url,
      api_key: payload.api_key,
      model_name: payload.model_name,
    });
    if (!sent) setFormAction(null);
  };
  const save = () => {
    const error = validate();
    if (error) { setFormError(error); return; }
    setFormError("");
    const payload = requestPayload();
    setFormAction("save");
    const sent = editingID
      ? props.onUpdate(payload)
      : props.onAdd({ ...payload, enabled: draft.enabled, is_default: draft.is_default });
    if (sent) closeForm();
    else setFormAction(null);
  };
  const remove = () => {
    if (!deleteTarget) return;
    const sent = props.onDelete(deleteTarget.id);
    if (sent) setDeleteTarget(null);
  };
  const setProtocol = (protocol: Protocol) => setDraft((current) => ({ ...current, protocol, auth_type: defaultAuthType(protocol), api_key: "" }));

  return (
    <View style={styles.root}>
      <View style={styles.headerCard}>
        <View style={styles.headingIcon}><Feather color={palette.brandLight} name="image" size={18} /></View>
        <View style={styles.flex}>
          <Text style={styles.title}>生图模型</Text>
          <Text style={styles.meta}>管理服务商、连接测试与默认模型。密钥只发送给 Agent，不会回显。</Text>
        </View>
        <Pressable accessibilityRole="button" disabled={!connected || loading} onPress={() => props.onRefresh()} style={({ pressed }) => buttonFeedback([styles.iconButton, (!connected || loading) && styles.disabled], pressed)}>
          <ButtonContent color={palette.ink1} loading={loading} text={loading ? "" : "刷新"} />
        </Pressable>
      </View>
      {!connected ? <Text style={styles.notice}>请先连接到在线 Agent，再管理生图模型。</Text> : null}
      {message ? <Text accessibilityLiveRegion="polite" style={messageError ? styles.error : styles.success}>{message}</Text> : null}
      {testResult ? <Text style={testResult.ok ? styles.success : styles.error}>{testResult.ok ? "测试通过" : "测试未通过"} · {testResult.latency_ms} ms{testResult.mime_type ? ` · ${testResult.mime_type}` : ""}</Text> : null}

      <View style={styles.rowActions}>
        <Pressable accessibilityRole="button" disabled={!connected || loading} onPress={openAdd} style={({ pressed }) => buttonFeedback([styles.primaryButton, (!connected || loading) && styles.disabled], pressed)}>
          <ButtonContent color={palette.inkInverse} text="添加生图模型" />
        </Pressable>
        <Text style={styles.meta}>{models.length} 个配置 · 默认：{models.find((item) => item.id === defaultModelID)?.name || "未设置"}</Text>
      </View>

      {models.length === 0 ? (
        <View style={styles.empty}><Feather color={palette.ink4} name="image" size={20} /><Text style={styles.meta}>还没有生图模型。添加后即可在聊天中生成图片。</Text></View>
      ) : models.map((model) => (
        <View key={model.id} style={styles.modelCard}>
          <View style={styles.cardTop}>
            <View style={styles.flex}>
              <View style={styles.nameLine}>
                <Text numberOfLines={1} style={styles.modelName}>{model.name || model.id}</Text>
                {model.is_default ? <Text style={styles.badge}>默认</Text> : null}
                {!model.enabled ? <Text style={styles.disabledBadge}>已停用</Text> : null}
              </View>
              <Text numberOfLines={1} style={styles.meta}>{model.protocol} · {model.model_name}</Text>
              <Text numberOfLines={1} style={styles.meta}>{model.base_url || "默认服务地址"}</Text>
              <Text style={styles.meta}>{model.has_api_key ? "已配置密钥" : model.auth_type === "google-adc" ? "使用 Agent Google ADC" : model.auth_type === "none" ? "无认证" : "尚无密钥"} · {model.supports_reference_images ? "支持参考图" : "仅文生图"}</Text>
            </View>
          </View>
          <View style={styles.cardActions}>
            {!model.is_default ? <Action label="设为默认" onPress={() => props.onSetDefault(model.id)} disabled={!connected || loading} styles={styles} feedback={buttonFeedback} /> : null}
            <Action label={model.enabled ? "停用" : "启用"} onPress={() => props.onSetEnabled(model.id, !model.enabled)} disabled={!connected || loading} styles={styles} feedback={buttonFeedback} />
            <Action label="编辑" onPress={() => openEdit(model)} disabled={!connected || loading} styles={styles} feedback={buttonFeedback} />
            <Action label="删除" danger onPress={() => setDeleteTarget(model)} disabled={!connected || loading} styles={styles} feedback={buttonFeedback} />
          </View>
        </View>
      ))}

      <ResponsiveFormModal
        buttonFeedback={buttonFeedback}
        onClose={closeForm}
        title={editingID ? "编辑生图模型" : "添加生图模型"}
        visible={formVisible}
        footer={(
          <View style={styles.modalFooter}>
            <Pressable disabled={!connected || loading} onPress={runTest} style={({ pressed }) => buttonFeedback([styles.secondaryButton, (!connected || loading) && styles.disabled], pressed)}>
              <ButtonContent color={palette.ink1} loading={loading && formAction === "test"} text="测试连接" />
            </Pressable>
            <Pressable disabled={!connected || loading} onPress={save} style={({ pressed }) => buttonFeedback([styles.primaryButton, (!connected || loading) && styles.disabled], pressed)}>
              <ButtonContent color={palette.inkInverse} loading={loading && formAction === "save"} text={editingID ? "保存修改" : "保存模型"} />
            </Pressable>
          </View>
        )}
      >
        <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={styles.formStack}>
          <Text style={styles.meta}>选择服务协议，再填写 Agent 访问图片服务所需的信息。</Text>
          <View style={styles.protocolGrid}>
            {protocols.map((item) => (
              <Pressable key={item.value} onPress={() => setProtocol(item.value)} style={({ pressed }) => buttonFeedback([styles.protocolCard, draft.protocol === item.value && styles.protocolActive], pressed)}>
                <Text style={styles.protocolTitle}>{item.label}</Text><Text style={styles.protocolHint}>{item.hint}</Text>
              </Pressable>
            ))}
          </View>
          {!editingID ? <Field label="配置 ID" value={draft.id} placeholder="例如 openai-image-main" onChangeText={(value) => updateDraft("id", value)} styles={styles} /> : <Text style={styles.meta}>配置 ID：{editingID}（不可修改）</Text>}
          <Field label="显示名称" value={draft.name} placeholder="例如 OpenAI Images" onChangeText={(value) => updateDraft("name", value)} styles={styles} />
          <Field label="服务商（可选）" value={draft.provider} placeholder="自动使用协议名称" onChangeText={(value) => updateDraft("provider", value)} styles={styles} />
          <Field label="Base URL" value={draft.base_url} placeholder={draft.protocol === "google-imagen" ? "https://LOCATION-aiplatform.googleapis.com/v1/projects/PROJECT/locations/LOCATION" : "留空使用服务商默认地址"} onChangeText={(value) => updateDraft("base_url", value)} styles={styles} />
          <Field label="服务商模型 ID" value={draft.model_name} placeholder={draft.protocol === "google-imagen" ? "例如 imagen-4.0-generate-001" : "填写服务商提供的图片模型 ID"} onChangeText={(value) => updateDraft("model_name", value)} styles={styles} />
          <Text style={styles.meta}>认证方式</Text>
          <View style={styles.authRow}>
            {allowedAuth.map(([value, label]) => <Pressable key={value} onPress={() => setDraft((current) => ({ ...current, auth_type: value, api_key: "" }))} style={({ pressed }) => buttonFeedback([styles.authChip, draft.auth_type === value && styles.protocolActive], pressed)}><Text style={styles.authText}>{label}</Text></Pressable>)}
          </View>
          {draft.auth_type !== "google-adc" && draft.auth_type !== "none" ? <Field label="API Key / Bearer Token" value={draft.api_key} placeholder={editingID ? "留空保留已保存密钥" : "输入密钥（仅发送给 Agent）"} onChangeText={(value) => updateDraft("api_key", value)} secure styles={styles} /> : null}
          {draft.auth_type === "google-adc" ? <Text style={styles.info}>使用 Agent 运行环境中的 Google Application Default Credentials（需要有 Vertex AI 权限）。</Text> : null}
          {editingID ? <Text style={styles.info}>编辑时密钥留空会保留现有值；若修改 Base URL，必须重新输入密钥。</Text> : (
            <Pressable onPress={() => updateDraft("enabled", !draft.enabled)} style={({ pressed }) => buttonFeedback([styles.toggleRow], pressed)}><Feather color={draft.enabled ? palette.ok : palette.ink4} name={draft.enabled ? "check-circle" : "circle"} size={18} /><Text style={styles.meta}>添加后立即启用</Text></Pressable>
          )}
          {!editingID ? <Pressable onPress={() => updateDraft("is_default", !draft.is_default)} style={({ pressed }) => buttonFeedback([styles.toggleRow], pressed)}><Feather color={draft.is_default ? palette.brandLight : palette.ink4} name={draft.is_default ? "check-circle" : "circle"} size={18} /><Text style={styles.meta}>设为默认生图模型</Text></Pressable> : null}
          {formError ? <Text style={styles.error}>{formError}</Text> : null}
          {message ? <Text accessibilityLiveRegion="polite" style={messageError ? styles.error : styles.success}>{message}</Text> : null}
          {testResult ? <Text style={testResult.ok ? styles.success : styles.error}>{testResult.ok ? "测试通过" : "测试未通过"} · {testResult.latency_ms} ms{testResult.mime_type ? ` · ${testResult.mime_type}` : ""}</Text> : null}
        </ScrollView>
      </ResponsiveFormModal>
      <ConfirmModal
        danger
        icon="trash-2"
        title="删除生图模型？"
        message={deleteTarget ? `将删除“${deleteTarget.name || deleteTarget.id}”配置。此操作不会影响已生成的图片。` : ""}
        confirmLabel="删除模型"
        onCancel={() => setDeleteTarget(null)}
        onConfirm={remove}
        pending={loading}
        visible={Boolean(deleteTarget)}
      />
    </View>
  );
}

function Field({ label, onChangeText, placeholder, secure, styles, value }: { label: string; onChangeText: (value: string) => void; placeholder: string; secure?: boolean; styles: ReturnType<typeof makeStyles>; value: string }) {
  return <View style={styles.field}><Text style={styles.fieldLabel}>{label}</Text><TextInput autoCapitalize="none" autoCorrect={false} onChangeText={onChangeText} placeholder={placeholder} placeholderTextColor="#8993a2" secureTextEntry={secure} style={styles.input} value={value} /></View>;
}
function Action({ danger, disabled, feedback, label, onPress, styles }: { danger?: boolean; disabled: boolean; feedback: ButtonFeedback; label: string; onPress: () => void; styles: ReturnType<typeof makeStyles> }) {
  return <Pressable accessibilityRole="button" disabled={disabled} onPress={onPress} style={({ pressed }) => feedback([styles.action, danger && styles.dangerAction, disabled && styles.disabled], pressed)}><Text style={[styles.actionText, danger && styles.dangerText]}>{label}</Text></Pressable>;
}
const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) => StyleSheet.create({
  root: { gap: 12 }, flex: { flex: 1, minWidth: 0 },
  headerCard: { alignItems: "center", backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.lg, borderWidth: 1, flexDirection: "row", gap: 11, padding: 13 },
  headingIcon: { alignItems: "center", backgroundColor: palette.brandWash, borderRadius: radius.md, height: 38, justifyContent: "center", width: 38 },
  title: { color: palette.ink1, fontSize: font.bodyLg, fontWeight: weight.semibold }, meta: { color: palette.ink3, fontSize: font.caption, lineHeight: 19 },
  iconButton: { alignItems: "center", borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, minHeight: 38, justifyContent: "center", minWidth: 54, paddingHorizontal: 10 },
  rowActions: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 10 },
  primaryButton: { alignItems: "center", backgroundColor: palette.brand, borderRadius: radius.md, justifyContent: "center", minHeight: 42, paddingHorizontal: 14 },
  secondaryButton: { alignItems: "center", backgroundColor: palette.surface2, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, justifyContent: "center", minHeight: 42, paddingHorizontal: 14 },
  disabled: { opacity: 0.5 },
  notice: { backgroundColor: palette.brandWash, borderColor: palette.brandDeep, borderRadius: radius.md, borderWidth: 1, color: palette.ink2, padding: 11 },
  error: { color: palette.err, fontSize: font.caption, lineHeight: 20 }, success: { color: palette.ok, fontSize: font.caption, lineHeight: 20 },
  empty: { alignItems: "center", backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.lg, borderWidth: 1, gap: 8, padding: 20 },
  modelCard: { backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.lg, borderWidth: 1, gap: 11, padding: 13 },
  cardTop: { alignItems: "flex-start", flexDirection: "row", gap: 10 }, nameLine: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 7 },
  modelName: { color: palette.ink1, fontSize: font.body, fontWeight: weight.semibold, maxWidth: "65%" },
  badge: { backgroundColor: palette.brandWash, borderRadius: 20, color: palette.brandLight, fontSize: font.label, overflow: "hidden", paddingHorizontal: 8, paddingVertical: 3 },
  disabledBadge: { backgroundColor: palette.well, borderRadius: 20, color: palette.ink3, fontSize: font.label, overflow: "hidden", paddingHorizontal: 8, paddingVertical: 3 },
  cardActions: { flexDirection: "row", flexWrap: "wrap", gap: 7 }, action: { backgroundColor: palette.surface2, borderColor: palette.hairline, borderRadius: radius.sm, borderWidth: 1, paddingHorizontal: 10, paddingVertical: 8 },
  actionText: { color: palette.ink2, fontSize: font.label, fontWeight: weight.medium }, dangerAction: { backgroundColor: palette.errWash, borderColor: palette.errBorder }, dangerText: { color: palette.err },
  formStack: { gap: 12, paddingBottom: 12 }, protocolGrid: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  protocolCard: { backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, flexGrow: 1, minWidth: 130, padding: 10 },
  protocolActive: { backgroundColor: palette.brandWash, borderColor: palette.brandDeep }, protocolTitle: { color: palette.ink1, fontSize: font.caption, fontWeight: weight.semibold }, protocolHint: { color: palette.ink3, fontSize: font.label, lineHeight: 16, marginTop: 3 },
  authRow: { flexDirection: "row", flexWrap: "wrap", gap: 7 }, authChip: { backgroundColor: palette.surface1, borderColor: palette.hairline, borderRadius: radius.sm, borderWidth: 1, paddingHorizontal: 10, paddingVertical: 9 }, authText: { color: palette.ink2, fontSize: font.caption },
  field: { gap: 5 }, fieldLabel: { color: palette.ink2, fontSize: font.caption, fontWeight: weight.medium }, input: { backgroundColor: palette.well, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, color: palette.ink1, fontSize: font.body, minHeight: 44, paddingHorizontal: 11, paddingVertical: 9 },
  info: { backgroundColor: palette.brandWash, borderRadius: radius.md, color: palette.ink2, fontSize: font.caption, lineHeight: 19, padding: 10 }, toggleRow: { alignItems: "center", flexDirection: "row", gap: 9, paddingVertical: 4 },
  modalFooter: { flexDirection: "row", flexWrap: "wrap", gap: 9, justifyContent: "flex-end" },
});
