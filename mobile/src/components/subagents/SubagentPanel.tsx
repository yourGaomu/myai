import { useMemo, useState } from "react";
import { LayoutAnimation, Platform, Pressable, StyleSheet, Switch, Text, TextInput, UIManager, View } from "react-native";

import type { SubagentDefinition, SubagentTask } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { ButtonContent } from "../common/ButtonContent";

type Props = {
  buttonFeedback: ButtonFeedback;
  definitions: SubagentDefinition[];
  message: string;
  onApplyTask: (taskID: string) => void;
  onCancelTask: (taskID: string) => void;
  onCheckTask: (taskID: string) => void;
  onCreateDefinition: (definition: Omit<SubagentDefinition, "id"> & { id?: string }) => boolean;
  onDeleteDefinition: (definitionID: string) => void;
  onDiscardTask: (taskID: string) => void;
  onResumeTask: (taskID: string) => void;
  onRefresh: () => void;
  onUpdateDefinition: (definition: SubagentDefinition) => boolean;
  pending: boolean;
  sessionID: string;
  tasks: SubagentTask[];
};

type DefinitionDraft = {
  id: string;
  name: string;
  description: string;
  systemPrompt: string;
  modelID: string;
  allowedTools: string;
  capabilityMode: SubagentDefinition["capability_mode"];
  isolationMode: SubagentDefinition["isolation_mode"];
  maxTurns: string;
  timeoutSeconds: string;
  enabled: boolean;
};

const emptyDraft: DefinitionDraft = {
  id: "",
  name: "",
  description: "",
  systemPrompt: "",
  modelID: "",
  allowedTools: "list_files, read_file, search_files",
  capabilityMode: "read_only",
  isolationMode: "direct",
  maxTurns: "8",
  timeoutSeconds: "300",
  enabled: true,
};

if (Platform.OS === "android") {
  UIManager.setLayoutAnimationEnabledExperimental?.(true);
}

const taskStatusLabels: Record<SubagentTask["status"], string> = {
  queued: "排队中",
  running: "执行中",
  waiting_subagents: "等待子任务",
  waiting_permission: "等待授权",
  succeeded: "已完成",
  failed: "失败",
  canceled: "已取消",
};

export function SubagentPanel({
  buttonFeedback,
  definitions,
  message,
  onApplyTask,
  onCancelTask,
  onCheckTask,
  onCreateDefinition,
  onDeleteDefinition,
  onDiscardTask,
  onResumeTask,
  onRefresh,
  onUpdateDefinition,
  pending,
  sessionID,
  tasks,
}: Props) {
  const [draft, setDraft] = useState<DefinitionDraft>(emptyDraft);
  const [showEditor, setShowEditor] = useState(false);
  const editing = Boolean(draft.id);
  const visibleTasks = useMemo(
    () => tasks.filter((task) => !sessionID || task.parent_session_id === sessionID),
    [sessionID, tasks],
  );

  const editDefinition = (definition: SubagentDefinition) => {
    setDraft({
      id: definition.id,
      name: definition.name,
      description: definition.description || "",
      systemPrompt: definition.system_prompt,
      modelID: definition.model_id || "",
      allowedTools: (definition.allowed_tools || []).join(", "),
      capabilityMode: definition.capability_mode,
      isolationMode: definition.isolation_mode,
      maxTurns: String(definition.max_turns || 8),
      timeoutSeconds: String(definition.timeout_seconds || 300),
      enabled: definition.enabled !== false,
    });
    setShowEditor(true);
  };

  const closeEditor = () => {
    setDraft(emptyDraft);
    setShowEditor(false);
  };

  const submit = () => {
    const value: SubagentDefinition = {
      id: draft.id,
      name: draft.name.trim(),
      description: draft.description.trim(),
      system_prompt: draft.systemPrompt.trim(),
      model_id: draft.modelID.trim(),
      allowed_tools: draft.allowedTools.split(",").map((item) => item.trim()).filter(Boolean),
      capability_mode: draft.capabilityMode,
      isolation_mode: draft.isolationMode,
      max_turns: Math.max(1, Number.parseInt(draft.maxTurns, 10) || 8),
      timeout_seconds: Math.max(1, Number.parseInt(draft.timeoutSeconds, 10) || 300),
      enabled: draft.enabled,
    };
    if (!value.name || !value.system_prompt) return;
    const sent = editing ? onUpdateDefinition(value) : onCreateDefinition(value);
    if (sent) closeEditor();
  };

  return (
    <View style={styles.stack}>
      <View style={styles.toolbar}>
        <View style={styles.flex}>
          <Text style={styles.title}>子智能体</Text>
          <Text style={styles.meta}>{definitions.length} 个配置 / 当前会话 {visibleTasks.length} 个任务</Text>
        </View>
        <Pressable disabled={pending} onPress={onRefresh} style={({ pressed }) => buttonFeedback([styles.secondaryButton, pending && styles.disabled], pressed)}>
          <ButtonContent loading={pending} text="刷新" />
        </Pressable>
        <Pressable
          disabled={pending}
          onPress={() => { setDraft(emptyDraft); setShowEditor(true); }}
          style={({ pressed }) => buttonFeedback([styles.primaryButton, pending && styles.disabled], pressed)}
        >
          <Text style={styles.primaryButtonText}>新建</Text>
        </Pressable>
      </View>

      {message ? <Text style={styles.notice}>{message}</Text> : null}

      {showEditor ? (
        <View style={styles.editor}>
          <View style={styles.rowBetween}>
            <Text style={styles.sectionTitle}>{editing ? "编辑配置" : "创建配置"}</Text>
            <Pressable onPress={closeEditor}><Text style={styles.link}>关闭</Text></Pressable>
          </View>
          <TextInput onChangeText={(value) => setDraft((current) => ({ ...current, name: value }))} placeholder="名称" placeholderTextColor="#776f66" style={styles.input} value={draft.name} />
          <TextInput onChangeText={(value) => setDraft((current) => ({ ...current, description: value }))} placeholder="用途说明" placeholderTextColor="#776f66" style={styles.input} value={draft.description} />
          <TextInput
            multiline
            onChangeText={(value) => setDraft((current) => ({ ...current, systemPrompt: value }))}
            placeholder="系统提示词"
            placeholderTextColor="#776f66"
            style={[styles.input, styles.promptInput]}
            textAlignVertical="top"
            value={draft.systemPrompt}
          />
          <TextInput onChangeText={(value) => setDraft((current) => ({ ...current, modelID: value }))} placeholder="模型 ID，留空继承父会话" placeholderTextColor="#776f66" style={styles.input} value={draft.modelID} />
          <TextInput onChangeText={(value) => setDraft((current) => ({ ...current, allowedTools: value }))} placeholder="工具白名单，以逗号分隔" placeholderTextColor="#776f66" style={styles.input} value={draft.allowedTools} />

          <Text style={styles.fieldLabel}>能力</Text>
          <View style={styles.segmentRow}>
            <Segment label="只读" selected={draft.capabilityMode === "read_only"} onPress={() => setDraft((current) => ({ ...current, capabilityMode: "read_only", isolationMode: "direct" }))} />
            <Segment label="读写" selected={draft.capabilityMode === "read_write"} onPress={() => setDraft((current) => ({ ...current, capabilityMode: "read_write", isolationMode: "snapshot" }))} />
            <Segment label="执行" selected={draft.capabilityMode === "execute"} onPress={() => setDraft((current) => ({ ...current, capabilityMode: "execute", isolationMode: "snapshot" }))} />
            <Segment label="全部" selected={draft.capabilityMode === "all"} onPress={() => setDraft((current) => ({ ...current, capabilityMode: "all", isolationMode: "snapshot" }))} />
          </View>
          <Text style={styles.fieldLabel}>工作区隔离</Text>
          <View style={styles.segmentRow}>
            <Segment label="直接只读" selected={draft.isolationMode === "direct"} onPress={() => setDraft((current) => ({ ...current, capabilityMode: "read_only", isolationMode: "direct" }))} />
            <Segment label="本地快照" selected={draft.isolationMode === "snapshot"} onPress={() => setDraft((current) => ({ ...current, isolationMode: "snapshot" }))} />
            <Segment label="OpenSandbox" selected={draft.isolationMode === "opensandbox"} onPress={() => setDraft((current) => ({ ...current, isolationMode: "opensandbox" }))} />
          </View>
          <View style={styles.row}>
            <TextInput keyboardType="number-pad" onChangeText={(value) => setDraft((current) => ({ ...current, maxTurns: value }))} placeholder="最大轮次" placeholderTextColor="#776f66" style={[styles.input, styles.flex]} value={draft.maxTurns} />
            <TextInput keyboardType="number-pad" onChangeText={(value) => setDraft((current) => ({ ...current, timeoutSeconds: value }))} placeholder="超时秒数" placeholderTextColor="#776f66" style={[styles.input, styles.flex]} value={draft.timeoutSeconds} />
          </View>
          <View style={styles.rowBetween}>
            <Text style={styles.fieldLabel}>启用</Text>
            <Switch onValueChange={(value) => setDraft((current) => ({ ...current, enabled: value }))} value={draft.enabled} />
          </View>
          <Pressable disabled={pending} onPress={submit} style={({ pressed }) => buttonFeedback([styles.primaryButton, pending && styles.disabled], pressed)}>
            <ButtonContent color="#ffffff" loading={pending} text={editing ? "保存配置" : "创建配置"} />
          </Pressable>
        </View>
      ) : null}

      <Text style={styles.sectionTitle}>配置</Text>
      {definitions.map((definition) => (
        <View key={definition.id} style={styles.item}>
          <View style={styles.rowBetween}>
            <View style={styles.flex}>
              <Text style={styles.itemTitle}>{definition.name}</Text>
              <Text style={styles.meta}>{definition.capability_mode} / {definition.isolation_mode} / v{definition.version || 1}</Text>
            </View>
            <Text style={[styles.badge, !definition.enabled && styles.badgeMuted]}>{definition.enabled ? "启用" : "停用"}</Text>
          </View>
          {definition.description ? <Text style={styles.body}>{definition.description}</Text> : null}
          <Text numberOfLines={2} style={styles.codeText}>{(definition.allowed_tools || []).join(", ") || "无工具"}</Text>
          {definition.source !== "builtin" ? (
            <View style={styles.actionRow}>
              <Pressable disabled={pending} onPress={() => editDefinition(definition)} style={({ pressed }) => buttonFeedback(styles.secondaryButton, pressed)}><Text style={styles.secondaryButtonText}>编辑</Text></Pressable>
              <Pressable disabled={pending} onPress={() => onDeleteDefinition(definition.id)} style={({ pressed }) => buttonFeedback(styles.dangerButton, pressed)}><Text style={styles.dangerText}>删除</Text></Pressable>
            </View>
          ) : null}
        </View>
      ))}

      <Text style={styles.sectionTitle}>后台任务</Text>
      {visibleTasks.length === 0 ? <Text style={styles.empty}>当前会话还没有子智能体任务。</Text> : visibleTasks.map((task) => (
        <TaskItem
          buttonFeedback={buttonFeedback}
          key={task.id}
          onApply={() => onApplyTask(task.id)}
          onCancel={() => onCancelTask(task.id)}
          onCheck={() => onCheckTask(task.id)}
          onDiscard={() => onDiscardTask(task.id)}
          onResume={() => onResumeTask(task.id)}
          pending={pending}
          task={task}
        />
      ))}
    </View>
  );
}

function Segment({ label, onPress, selected }: { label: string; onPress: () => void; selected: boolean }) {
  return <Pressable onPress={onPress} style={[styles.segment, selected && styles.segmentActive]}><Text style={[styles.segmentText, selected && styles.segmentTextActive]}>{label}</Text></Pressable>;
}

function TaskItem({ buttonFeedback, onApply, onCancel, onCheck, onDiscard, onResume, pending, task }: {
  buttonFeedback: ButtonFeedback;
  onApply: () => void;
  onCancel: () => void;
  onCheck: () => void;
  onDiscard: () => void;
  onResume: () => void;
  pending: boolean;
  task: SubagentTask;
}) {
  const [expanded, setExpanded] = useState(false);
  const running = task.status === "queued" || task.status === "running" || task.status === "waiting_subagents" || task.status === "waiting_permission";
  const pendingChanges = task.change_set?.status === "pending";
  const canResume = task.unread && (task.status === "succeeded" || task.status === "failed");
  const summary = task.error_message || task.result || (running ? "子智能体正在执行。" : "子智能体未返回文本结果。");
  const toggleExpanded = () => {
    LayoutAnimation.configureNext(LayoutAnimation.Presets.easeInEaseOut);
    setExpanded((current) => !current);
  };
  return (
    <View style={[styles.item, styles.taskItem]}>
      <Pressable
        accessibilityLabel={expanded ? "收起任务详情" : "展开任务详情"}
        accessibilityRole="button"
        accessibilityState={{ expanded }}
        onPress={toggleExpanded}
        style={({ pressed }) => [styles.taskHeader, pressed && styles.taskHeaderPressed]}
      >
        <View style={styles.flex}>
          <View style={styles.taskTitleRow}>
            <Text numberOfLines={2} style={[styles.itemTitle, styles.taskTitle]}>{task.title}</Text>
            {canResume ? <View style={styles.unreadDot} /> : null}
          </View>
          <Text style={styles.meta}>{task.definition_id} · {taskStatusLabels[task.status]}</Text>
        </View>
        <View style={styles.taskHeaderSide}>
          <Text style={styles.badge}>{task.change_set?.files?.length || 0} 文件</Text>
          <View style={styles.drawerIcon}>
            <Text style={styles.drawerIconText}>{expanded ? "▴" : "▾"}</Text>
          </View>
        </View>
      </Pressable>

      {expanded ? (
        <View style={styles.taskDrawer}>
          {task.instruction ? (
            <View style={styles.taskDetail}>
              <Text style={styles.fieldLabel}>任务要求</Text>
              <Text style={styles.body}>{task.instruction}</Text>
            </View>
          ) : null}
          <View style={styles.taskDetail}>
            <Text style={styles.fieldLabel}>执行结果</Text>
            {task.result ? <Text style={styles.body}>{task.result}</Text> : (
              <Text style={styles.emptyResult}>{running ? "子智能体正在执行。" : "子智能体未返回文本结果。"}</Text>
            )}
          </View>
          {task.error_message ? <Text style={styles.errorText}>{task.error_message}</Text> : null}
          {(task.change_set?.files || []).map((file) => <Text key={file.path} style={styles.codeText}>{file.change_type}  {file.path}</Text>)}
          <View style={styles.actionRow}>
            {canResume ? <Pressable disabled={pending} onPress={onResume} style={({ pressed }) => buttonFeedback(styles.primaryButton, pressed)}><ButtonContent color="#ffffff" loading={pending} text="继续主任务" /></Pressable> : null}
            <Pressable disabled={pending} onPress={onCheck} style={({ pressed }) => buttonFeedback(styles.secondaryButton, pressed)}><ButtonContent loading={pending} text="刷新状态" /></Pressable>
            {running ? <Pressable disabled={pending} onPress={onCancel} style={({ pressed }) => buttonFeedback(styles.dangerButton, pressed)}><ButtonContent color="#8b2822" loading={pending} text="取消" /></Pressable> : null}
            {pendingChanges ? <Pressable disabled={pending} onPress={onApply} style={({ pressed }) => buttonFeedback(styles.primaryButton, pressed)}><ButtonContent color="#ffffff" loading={pending} text="应用变更" /></Pressable> : null}
            {pendingChanges ? <Pressable disabled={pending} onPress={onDiscard} style={({ pressed }) => buttonFeedback(styles.dangerButton, pressed)}><ButtonContent color="#8b2822" loading={pending} text="丢弃" /></Pressable> : null}
          </View>
        </View>
      ) : (
        <View style={styles.taskPreview}>
          <Text numberOfLines={2} style={[styles.taskPreviewText, task.error_message && styles.taskPreviewError]}>{summary}</Text>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  actionRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 },
  badge: { backgroundColor: "#dcebd8", borderRadius: 4, color: "#214222", fontSize: 11, fontWeight: "800", paddingHorizontal: 7, paddingVertical: 4 },
  badgeMuted: { backgroundColor: "#ded8cf", color: "#5e574f" },
  body: { color: "#38332d", fontSize: 13, lineHeight: 19, marginTop: 7 },
  codeText: { color: "#575048", fontFamily: "monospace", fontSize: 11, marginTop: 6 },
  dangerButton: { alignItems: "center", borderColor: "#9f352d", borderRadius: 5, borderWidth: 1, justifyContent: "center", minHeight: 36, paddingHorizontal: 12 },
  dangerText: { color: "#8b2822", fontSize: 12, fontWeight: "800" },
  disabled: { opacity: 0.5 },
  drawerIcon: { alignItems: "center", backgroundColor: "#eee6da", borderRadius: 4, height: 28, justifyContent: "center", width: 28 },
  drawerIconText: { color: "#39342e", fontSize: 15, fontWeight: "900", lineHeight: 18 },
  editor: { borderColor: "#1c1916", borderRadius: 6, borderWidth: 2, gap: 9, padding: 12 },
  empty: { color: "#756d64", fontSize: 13, paddingVertical: 12 },
  emptyResult: { color: "#756d64", fontSize: 12, lineHeight: 18, marginTop: 5 },
  errorText: { color: "#9f352d", fontSize: 12, marginTop: 7 },
  fieldLabel: { color: "#38332d", fontSize: 12, fontWeight: "800" },
  flex: { flex: 1 },
  input: { backgroundColor: "#fffdf8", borderColor: "#b8aa98", borderRadius: 5, borderWidth: 1, color: "#171411", fontSize: 13, minHeight: 42, paddingHorizontal: 11, paddingVertical: 9 },
  item: { backgroundColor: "#fffdf8", borderColor: "#c7b9a6", borderRadius: 6, borderWidth: 1, padding: 12 },
  itemTitle: { color: "#171411", fontSize: 14, fontWeight: "900" },
  link: { color: "#315d48", fontSize: 12, fontWeight: "800" },
  meta: { color: "#756d64", fontSize: 11, lineHeight: 16 },
  notice: { backgroundColor: "#e6efe4", borderRadius: 5, color: "#2c5030", fontSize: 12, padding: 9 },
  primaryButton: { alignItems: "center", backgroundColor: "#1d5c45", borderRadius: 5, justifyContent: "center", minHeight: 38, paddingHorizontal: 13 },
  primaryButtonText: { color: "#ffffff", fontSize: 12, fontWeight: "900" },
  promptInput: { minHeight: 110 },
  row: { flexDirection: "row", gap: 9 },
  rowBetween: { alignItems: "center", flexDirection: "row", gap: 10, justifyContent: "space-between" },
  secondaryButton: { alignItems: "center", backgroundColor: "#eee6da", borderRadius: 5, justifyContent: "center", minHeight: 36, paddingHorizontal: 12 },
  secondaryButtonText: { color: "#39342e", fontSize: 12, fontWeight: "800" },
  sectionTitle: { color: "#171411", fontSize: 13, fontWeight: "900", marginTop: 3 },
  segment: { alignItems: "center", backgroundColor: "#eee6da", borderRadius: 4, flex: 1, justifyContent: "center", minHeight: 36, paddingHorizontal: 7 },
  segmentActive: { backgroundColor: "#d4e8dc", borderColor: "#1d5c45", borderWidth: 1 },
  segmentRow: { flexDirection: "row", gap: 6 },
  segmentText: { color: "#5d554c", fontSize: 11, fontWeight: "800" },
  segmentTextActive: { color: "#173e2e" },
  stack: { gap: 10 },
  taskDrawer: { borderTopColor: "#ded4c7", borderTopWidth: 1, padding: 12, paddingTop: 3 },
  taskDetail: { marginTop: 9 },
  taskHeader: { alignItems: "center", flexDirection: "row", gap: 10, minHeight: 66, padding: 12 },
  taskHeaderPressed: { backgroundColor: "#f5efe5" },
  taskHeaderSide: { alignItems: "center", flexDirection: "row", gap: 7 },
  taskItem: { overflow: "hidden", padding: 0 },
  taskPreview: { borderTopColor: "#ede4d8", borderTopWidth: 1, paddingHorizontal: 12, paddingVertical: 9 },
  taskPreviewError: { color: "#9f352d" },
  taskPreviewText: { color: "#675f56", fontSize: 12, lineHeight: 17 },
  taskTitle: { flexShrink: 1 },
  taskTitleRow: { alignItems: "center", flexDirection: "row", gap: 7 },
  title: { color: "#171411", fontSize: 17, fontWeight: "900" },
  toolbar: { alignItems: "center", flexDirection: "row", gap: 8 },
  unreadDot: { backgroundColor: "#1d5c45", borderRadius: 4, height: 7, width: 7 },
});
