import { useMemo, useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import type { AIMemory, AIMemoryCandidate, AIMemoryInput, AIMemoryKind, AIMemoryListPayload, AIMemoryScope } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { ButtonContent } from "../common/ButtonContent";

export type AIMemoryPanelProps = {
  buttonFeedback: ButtonFeedback;
  candidates: AIMemoryCandidate[];
  memories: AIMemory[];
  message: string;
  onApproveCandidate: (candidate: AIMemoryCandidate, memoryID?: string) => boolean;
  onCreateMemory: (memory: AIMemoryInput) => boolean;
  onDeleteMemory: (memoryID: string) => boolean;
  onRefreshCandidates: () => boolean;
  onRefreshMemories: (filter?: AIMemoryListPayload) => boolean;
  onRejectCandidate: (candidate: AIMemoryCandidate) => boolean;
  onRestoreMemory: (memoryID: string) => boolean;
  onUpdateMemory: (memoryID: string, memory: AIMemoryInput) => boolean;
  pending: boolean;
};

type ViewTab = "memories" | "candidates";
type Draft = AIMemoryInput & { tagsText: string };

const emptyDraft: Draft = {
  title: "",
  kind: "experience",
  scope: { type: "global" },
  tagsText: "",
  content: { goal: "", approach: "", result: "", pain_points: "", root_cause: "", lessons: "", verification: "", applicable_context: "" },
  confidence: 1,
};

export function AIMemoryPanel({
  buttonFeedback,
  candidates,
  memories,
  message,
  onApproveCandidate,
  onCreateMemory,
  onDeleteMemory,
  onRefreshCandidates,
  onRefreshMemories,
  onRejectCandidate,
  onRestoreMemory,
  onUpdateMemory,
  pending,
}: AIMemoryPanelProps) {
  const [tab, setTab] = useState<ViewTab>("memories");
  const [query, setQuery] = useState("");
  const [expandedID, setExpandedID] = useState("");
  const [editingID, setEditingID] = useState("");
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [showEditor, setShowEditor] = useState(false);
  const [includeDeleted, setIncludeDeleted] = useState(false);

  const activeCount = useMemo(() => memories.filter((memory) => memory.status !== "deleted").length, [memories]);

  const filtered = useMemo(() => {
    const value = query.trim().toLowerCase();
    return memories.filter((memory) => {
      if (!includeDeleted && memory.status === "deleted") return false;
      if (!value) return true;
      const revision = currentRevision(memory);
      return [memory.title, ...(memory.tags || []), revision?.content.goal, revision?.content.approach, revision?.content.lessons]
        .filter(Boolean)
        .join("\n")
        .toLowerCase()
        .includes(value);
    });
  }, [includeDeleted, memories, query]);

  const refresh = () => {
    if (tab === "memories") onRefreshMemories({ include_deleted: includeDeleted, text: query.trim() || undefined, limit: 100 });
    else onRefreshCandidates();
  };

  const beginCreate = () => {
    setEditingID("");
    setDraft(emptyDraft);
    setShowEditor(true);
  };

  const beginEdit = (memory: AIMemory) => {
    const revision = currentRevision(memory);
    if (!revision) return;
    setEditingID(memory.id);
    setDraft({
      title: memory.title,
      kind: memory.kind,
      scope: memory.scope,
      tags: memory.tags,
      tagsText: (memory.tags || []).join(", "),
      content: { ...revision.content },
      confidence: revision.confidence,
    });
    setShowEditor(true);
  };

  const submit = () => {
    if (!draft.title.trim() || !draft.content.goal.trim()) return;
    if (!draft.content.approach?.trim() && !draft.content.lessons?.trim()) return;
    const input: AIMemoryInput = {
      title: draft.title.trim(),
      kind: draft.kind,
      scope: normalizeScope(draft.scope),
      tags: draft.tagsText.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean),
      content: trimContent(draft.content),
      confidence: draft.confidence,
    };
    const sent = editingID ? onUpdateMemory(editingID, input) : onCreateMemory(input);
    if (sent) setShowEditor(false);
  };

  return (
    <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <View style={styles.headerRow}>
        <View style={styles.flex}>
          <Text style={styles.eyebrow}>AI MEMORY</Text>
          <Text style={styles.title}>经验记忆</Text>
          <Text style={styles.meta}>{activeCount} 条有效记录 · {candidates.length} 条待审核</Text>
        </View>
        <Pressable disabled={pending} onPress={refresh} style={({ pressed }) => buttonFeedback([styles.outlineButton, pending && styles.disabled], pressed)}>
          <ButtonContent loading={pending} text="刷新" />
        </Pressable>
      </View>

      <View style={styles.segmented}>
        <Segment active={tab === "memories"} label="有效记忆" onPress={() => setTab("memories")} buttonFeedback={buttonFeedback} />
        <Segment active={tab === "candidates"} label={`待审核 ${candidates.length}`} onPress={() => setTab("candidates")} buttonFeedback={buttonFeedback} />
      </View>

      {message ? <Text style={styles.status}>{message}</Text> : null}

      {tab === "memories" ? (
        <>
          <View style={styles.toolbar}>
            <TextInput
              onChangeText={setQuery}
              placeholder="搜索目标、方案或标签"
              placeholderTextColor="#777169"
              style={[styles.input, styles.flex]}
              value={query}
            />
            <Pressable disabled={pending} onPress={beginCreate} style={({ pressed }) => buttonFeedback(styles.primaryButton, pressed)}>
              <Text style={styles.primaryText}>+ 新建</Text>
            </Pressable>
          </View>
          <Pressable onPress={() => { const next = !includeDeleted; setIncludeDeleted(next); onRefreshMemories({ include_deleted: next, limit: 100 }); }} style={({ pressed }) => buttonFeedback(styles.filterToggle, pressed)}>
            <Text style={styles.filterToggleText}>{includeDeleted ? "隐藏已删除" : "查看已删除"}</Text>
          </Pressable>

          {showEditor ? (
            <MemoryEditor
              buttonFeedback={buttonFeedback}
              draft={draft}
              editing={Boolean(editingID)}
              onCancel={() => setShowEditor(false)}
              onChange={setDraft}
              onSubmit={submit}
              pending={pending}
            />
          ) : null}

          {filtered.map((memory) => {
            const revision = currentRevision(memory);
            const expanded = expandedID === memory.id;
            return (
              <View key={memory.id} style={[styles.memoryCard, memory.status === "deleted" && styles.deletedCard]}>
                <Pressable onPress={() => setExpandedID(expanded ? "" : memory.id)} style={({ pressed }) => buttonFeedback(styles.memoryHeader, pressed)}>
                  <View style={styles.flex}>
                    <View style={styles.labelRow}>
                      <Text style={styles.memoryTitle}>{memory.title}</Text>
                      <Text style={[styles.kindLabel, kindStyle(memory.kind)]}>{kindLabel(memory.kind)}</Text>
                    </View>
                    <Text numberOfLines={expanded ? undefined : 2} style={styles.goal}>{revision?.content.goal || "无目标描述"}</Text>
                    <Text style={styles.meta}>v{memory.current_version} · 可信度 {Math.round((revision?.confidence || 0) * 100)}% · 使用 {memory.use_count || 0} 次</Text>
                  </View>
                  <Text style={styles.chevron}>{expanded ? "−" : "+"}</Text>
                </Pressable>
                <View style={styles.tagRow}>{(memory.tags || []).map((tag) => <Text key={tag} style={styles.tag}>#{tag}</Text>)}</View>
                {expanded && revision ? (
                  <View style={styles.details}>
                    <MemoryContentView content={revision.content} />
                    <Text style={styles.meta}>范围：{scopeLabel(memory.scope)} · 来源：{revision.author === "human" ? "人工" : "模型"}</Text>
                    <View style={styles.actions}>
                      {memory.status === "deleted" ? (
                        <Action label="恢复" onPress={() => onRestoreMemory(memory.id)} buttonFeedback={buttonFeedback} />
                      ) : (
                        <>
                          <Action label="编辑" onPress={() => beginEdit(memory)} buttonFeedback={buttonFeedback} />
                          <Action danger label="删除" onPress={() => onDeleteMemory(memory.id)} buttonFeedback={buttonFeedback} />
                        </>
                      )}
                    </View>
                  </View>
                ) : null}
              </View>
            );
          })}
          {!pending && filtered.length === 0 ? <Text style={styles.empty}>还没有符合条件的 AI 记忆。</Text> : null}
        </>
      ) : (
        <>
          {pending && candidates.length === 0 ? <ActivityIndicator color="#1d6b52" /> : null}
          {candidates.map((candidate) => (
            <View key={candidate.id} style={styles.memoryCard}>
              <View style={styles.labelRow}>
                <Text style={styles.memoryTitle}>{candidate.title}</Text>
                <Text style={[styles.kindLabel, kindStyle(candidate.kind)]}>{kindLabel(candidate.kind)}</Text>
              </View>
              <Text style={styles.goal}>{candidate.content.goal}</Text>
              <MemoryContentView content={candidate.content} />
              <View style={styles.tagRow}>{(candidate.tags || []).map((tag) => <Text key={tag} style={styles.tag}>#{tag}</Text>)}</View>
              <Text style={styles.meta}>可信度 {Math.round(candidate.confidence * 100)}% · {candidate.sources?.[0]?.agent_run_id ? "来自 AgentRun" : "自动提取"}</Text>
              <View style={styles.actions}>
                <Action label="通过" onPress={() => onApproveCandidate(candidate)} buttonFeedback={buttonFeedback} />
                <Action danger label="拒绝" onPress={() => onRejectCandidate(candidate)} buttonFeedback={buttonFeedback} />
              </View>
            </View>
          ))}
          {!pending && candidates.length === 0 ? <Text style={styles.empty}>没有待审核候选。</Text> : null}
        </>
      )}
    </ScrollView>
  );
}

function MemoryEditor({ buttonFeedback, draft, editing, onCancel, onChange, onSubmit, pending }: {
  buttonFeedback: ButtonFeedback;
  draft: Draft;
  editing: boolean;
  onCancel: () => void;
  onChange: (draft: Draft) => void;
  onSubmit: () => void;
  pending: boolean;
}) {
  const setContent = (key: keyof AIMemoryInput["content"], value: string) => onChange({ ...draft, content: { ...draft.content, [key]: value } });
  return (
    <View style={styles.editor}>
      <View style={styles.headerRow}>
        <Text style={styles.sectionTitle}>{editing ? "编辑记忆" : "新建记忆"}</Text>
        <Action label="关闭" onPress={onCancel} buttonFeedback={buttonFeedback} />
      </View>
      <TextInput onChangeText={(title) => onChange({ ...draft, title })} placeholder="标题" placeholderTextColor="#777169" style={styles.input} value={draft.title} />
      <View style={styles.segmented}>
        {(["experience", "failure", "decision", "preference"] as AIMemoryKind[]).map((kind) => (
          <Segment active={draft.kind === kind} buttonFeedback={buttonFeedback} key={kind} label={kindLabel(kind)} onPress={() => onChange({ ...draft, kind })} />
        ))}
      </View>
      <EditorField label="目标" onChange={(value) => setContent("goal", value)} value={draft.content.goal} />
      <EditorField label="适用条件" onChange={(value) => setContent("applicable_context", value)} value={draft.content.applicable_context || ""} />
      <EditorField label="方案" onChange={(value) => setContent("approach", value)} value={draft.content.approach || ""} />
      <EditorField label="结果" onChange={(value) => setContent("result", value)} value={draft.content.result || ""} />
      <EditorField label="痛点" onChange={(value) => setContent("pain_points", value)} value={draft.content.pain_points || ""} />
      <EditorField label="错误原因" onChange={(value) => setContent("root_cause", value)} value={draft.content.root_cause || ""} />
      <EditorField label="经验结论" onChange={(value) => setContent("lessons", value)} value={draft.content.lessons || ""} />
      <EditorField label="验证方式" onChange={(value) => setContent("verification", value)} value={draft.content.verification || ""} />
      <TextInput onChangeText={(tagsText) => onChange({ ...draft, tagsText })} placeholder="标签，用逗号分隔" placeholderTextColor="#777169" style={styles.input} value={draft.tagsText} />
      <Pressable disabled={pending} onPress={onSubmit} style={({ pressed }) => buttonFeedback([styles.primaryButton, pending && styles.disabled], pressed)}>
        <ButtonContent loading={pending} text={editing ? "保存新版本" : "创建记忆"} />
      </Pressable>
    </View>
  );
}

function EditorField({ label, onChange, value }: { label: string; onChange: (value: string) => void; value: string }) {
  return (
    <View>
      <Text style={styles.fieldLabel}>{label}</Text>
      <TextInput multiline onChangeText={onChange} placeholder={label} placeholderTextColor="#777169" style={[styles.input, styles.textarea]} value={value} />
    </View>
  );
}

function MemoryContentView({ content }: { content: AIMemoryInput["content"] }) {
  const rows = [
    ["适用条件", content.applicable_context], ["方案", content.approach], ["结果", content.result],
    ["痛点", content.pain_points], ["错误原因", content.root_cause], ["经验", content.lessons], ["验证", content.verification],
  ];
  return <View style={styles.detailGrid}>{rows.filter(([, value]) => value).map(([label, value]) => <View key={label}><Text style={styles.fieldLabel}>{label}</Text><Text style={styles.detailText}>{value}</Text></View>)}</View>;
}

function Segment({ active, buttonFeedback, label, onPress }: { active: boolean; buttonFeedback: ButtonFeedback; label: string; onPress: () => void }) {
  return <Pressable onPress={onPress} style={({ pressed }) => buttonFeedback([styles.segment, active && styles.segmentActive], pressed)}><Text style={[styles.segmentText, active && styles.segmentTextActive]}>{label}</Text></Pressable>;
}

function Action({ buttonFeedback, danger = false, label, onPress }: { buttonFeedback: ButtonFeedback; danger?: boolean; label: string; onPress: () => void }) {
  return <Pressable onPress={onPress} style={({ pressed }) => buttonFeedback([styles.actionButton, danger && styles.dangerButton], pressed)}><Text style={[styles.actionText, danger && styles.dangerText]}>{label}</Text></Pressable>;
}

function currentRevision(memory: AIMemory) {
  return memory.revisions.find((revision) => revision.version === memory.current_version) || memory.revisions[memory.revisions.length - 1];
}

function trimContent(content: AIMemoryInput["content"]): AIMemoryInput["content"] {
  return Object.fromEntries(Object.entries(content).map(([key, value]) => [key, value?.trim() || ""])) as AIMemoryInput["content"];
}

function normalizeScope(scope: AIMemoryScope): AIMemoryScope {
  return scope.type === "global" ? { type: "global" } : { type: scope.type, key: scope.key?.trim() || "" };
}

function kindLabel(kind: AIMemoryKind) {
  return ({ experience: "经验", failure: "失败", decision: "决策", preference: "偏好" } as const)[kind];
}

function scopeLabel(scope: AIMemoryScope) {
  return scope.type === "global" ? "全局" : `${scope.type}:${scope.key || "未指定"}`;
}

function kindStyle(kind: AIMemoryKind) {
  if (kind === "failure") return styles.failureLabel;
  if (kind === "decision") return styles.decisionLabel;
  if (kind === "preference") return styles.preferenceLabel;
  return styles.experienceLabel;
}

const styles = StyleSheet.create({
  content: { padding: 16, paddingBottom: 120, gap: 12 },
  flex: { flex: 1 },
  headerRow: { alignItems: "center", flexDirection: "row", gap: 12, justifyContent: "space-between" },
  eyebrow: { color: "#1d6b52", fontSize: 11, fontWeight: "800" },
  title: { color: "#1d211e", fontSize: 24, fontWeight: "800" },
  sectionTitle: { color: "#1d211e", fontSize: 17, fontWeight: "800" },
  meta: { color: "#716b63", fontSize: 12, marginTop: 3 },
  segmented: { backgroundColor: "#e8e5df", borderRadius: 7, flexDirection: "row", gap: 3, padding: 3 },
  segment: { alignItems: "center", borderRadius: 5, flex: 1, minHeight: 36, justifyContent: "center", paddingHorizontal: 8 },
  segmentActive: { backgroundColor: "#ffffff", borderColor: "#d4d0c8", borderWidth: 1 },
  segmentText: { color: "#706a62", fontSize: 13, fontWeight: "700" },
  segmentTextActive: { color: "#1d211e" },
  toolbar: { flexDirection: "row", gap: 8 },
  input: { backgroundColor: "#ffffff", borderColor: "#d8d3ca", borderRadius: 6, borderWidth: 1, color: "#20231f", fontSize: 14, minHeight: 42, paddingHorizontal: 12, paddingVertical: 9 },
  textarea: { minHeight: 76, textAlignVertical: "top" },
  primaryButton: { alignItems: "center", backgroundColor: "#1d6b52", borderRadius: 6, justifyContent: "center", minHeight: 42, paddingHorizontal: 15 },
  primaryText: { color: "#ffffff", fontSize: 14, fontWeight: "800" },
  outlineButton: { alignItems: "center", borderColor: "#a8a197", borderRadius: 6, borderWidth: 1, justifyContent: "center", minHeight: 38, paddingHorizontal: 12 },
  disabled: { opacity: 0.5 },
  status: { backgroundColor: "#edf5f0", borderLeftColor: "#1d6b52", borderLeftWidth: 3, color: "#345348", padding: 10 },
  memoryCard: { backgroundColor: "#ffffff", borderColor: "#d9d5cd", borderRadius: 7, borderWidth: 1, padding: 13 },
  deletedCard: { opacity: 0.62 },
  memoryHeader: { alignItems: "flex-start", flexDirection: "row", gap: 10 },
  labelRow: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 7 },
  memoryTitle: { color: "#1f231f", flexShrink: 1, fontSize: 16, fontWeight: "800" },
  kindLabel: { borderRadius: 4, fontSize: 11, fontWeight: "800", overflow: "hidden", paddingHorizontal: 6, paddingVertical: 3 },
  experienceLabel: { backgroundColor: "#dfeee8", color: "#1d6b52" },
  failureLabel: { backgroundColor: "#f8dfdc", color: "#a13d33" },
  decisionLabel: { backgroundColor: "#e4e8f3", color: "#425b8d" },
  preferenceLabel: { backgroundColor: "#f1e7cf", color: "#735d24" },
  goal: { color: "#454a45", fontSize: 14, lineHeight: 20, marginTop: 8 },
  chevron: { color: "#666159", fontSize: 20, width: 24 },
  tagRow: { flexDirection: "row", flexWrap: "wrap", gap: 7, marginTop: 9 },
  tag: { color: "#1d6b52", fontSize: 12, fontWeight: "700" },
  details: { borderTopColor: "#ece9e3", borderTopWidth: 1, gap: 12, marginTop: 12, paddingTop: 12 },
  detailGrid: { gap: 10 },
  fieldLabel: { color: "#6b655d", fontSize: 11, fontWeight: "800", marginBottom: 4 },
  detailText: { color: "#303530", fontSize: 14, lineHeight: 21 },
  actions: { flexDirection: "row", gap: 8, justifyContent: "flex-end" },
  actionButton: { borderColor: "#9caa9f", borderRadius: 5, borderWidth: 1, minHeight: 34, justifyContent: "center", paddingHorizontal: 12 },
  actionText: { color: "#275f4c", fontSize: 13, fontWeight: "800" },
  dangerButton: { borderColor: "#d3aaa4" },
  dangerText: { color: "#9b4037" },
  editor: { backgroundColor: "#f4f2ed", borderColor: "#d7d2c8", borderRadius: 7, borderWidth: 1, gap: 10, padding: 13 },
  empty: { color: "#777169", paddingVertical: 28, textAlign: "center" },
  filterToggle: { alignSelf: "flex-start", minHeight: 32, justifyContent: "center", paddingHorizontal: 2 },
  filterToggleText: { color: "#466f60", fontSize: 13, fontWeight: "700" },
});
