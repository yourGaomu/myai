import { useMemo, useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type {
  AIMemory,
  AIMemoryCandidate,
  AIMemoryDreamRun,
  AIMemoryExtractionJob,
  AIMemoryInput,
  AIMemoryKind,
  AIMemoryListPayload,
  AIMemoryRevision,
  AIMemoryScope,
} from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { ButtonContent } from "../common/ButtonContent";
import { ResponsiveFormModal } from "../common/ResponsiveFormModal";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

export type AIMemoryPanelProps = {
  buttonFeedback: ButtonFeedback;
  candidates: AIMemoryCandidate[];
  dreamRuns: AIMemoryDreamRun[];
  extractionJobs: AIMemoryExtractionJob[];
  memories: AIMemory[];
  message: string;
  onApproveCandidate: (candidate: AIMemoryCandidate, memoryID?: string) => boolean;
  onCreateMemory: (memory: AIMemoryInput) => boolean;
  onDeleteMemory: (memoryID: string) => boolean;
  onRefreshCandidates: () => boolean;
  onRefreshDreamRuns: () => boolean;
  onRefreshExtractionJobs: () => boolean;
  onRefreshMemories: (filter?: AIMemoryListPayload) => boolean;
  onRejectCandidate: (candidate: AIMemoryCandidate) => boolean;
  onRestoreMemory: (memoryID: string) => boolean;
  onRetryExtractionJob: (job: AIMemoryExtractionJob) => boolean;
  onRunDream: () => boolean;
  onUpdateMemory: (memoryID: string, memory: AIMemoryInput) => boolean;
  pending: boolean;
};

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
  extractionJobs,
  memories,
  message,
  onApproveCandidate,
  onCreateMemory,
  onDeleteMemory,
  onRefreshExtractionJobs,
  onRejectCandidate,
  onRestoreMemory,
  onRetryExtractionJob,
  onRunDream,
  onUpdateMemory,
  pending,
}: AIMemoryPanelProps) {
  const [expandedID, setExpandedID] = useState("");
  const [showCandidates, setShowCandidates] = useState(false);
  const [editingID, setEditingID] = useState("");
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [showEditor, setShowEditor] = useState(false);
  const [mergeCandidate, setMergeCandidate] = useState<AIMemoryCandidate | null>(null);
  const [mergeMemoryID, setMergeMemoryID] = useState("");
  const [mergeQuery, setMergeQuery] = useState("");

  const activeMemories = useMemo(
    () => memories.filter((memory) => memory.status !== "deleted"),
    [memories],
  );

  const mergeTargets = useMemo(() => {
    const value = mergeQuery.trim().toLowerCase();
    return memories.filter((memory) => {
      if (memory.status !== "active") return false;
      if (!value) return true;
      const revision = currentRevision(memory);
      return [memory.title, ...(memory.tags || []), revision?.content.goal, revision?.content.approach]
        .filter(Boolean)
        .join("\n")
        .toLowerCase()
        .includes(value);
    });
  }, [memories, mergeQuery]);

  const beginCreate = () => {
    setEditingID("");
    setDraft(emptyDraft);
    setShowEditor(true);
  };

  const beginMerge = (candidate: AIMemoryCandidate) => {
    setMergeCandidate(candidate);
    setMergeMemoryID("");
    setMergeQuery("");
  };

  const confirmMerge = () => {
    if (!mergeCandidate || !mergeMemoryID || pending) return;
    if (onApproveCandidate(mergeCandidate, mergeMemoryID)) {
      setMergeCandidate(null);
    }
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
    if (!draft.title.trim() && !draft.content.goal.trim()) return;
    const input: AIMemoryInput = {
      title: draft.title.trim() || draft.content.goal.slice(0, 30),
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
    <>
      <View style={styles.content}>
        {/* 顶部标题行: 已沉淀用户记忆 + [+ 添加记忆] 按钮 */}
        <View style={styles.headerRow}>
          <Text style={styles.headerTitle}>已沉淀用户记忆</Text>
          <Pressable
            disabled={pending}
            onPress={beginCreate}
            style={({ pressed }) => buttonFeedback(styles.addBtn, pressed)}
          >
            <Text style={styles.addBtnText}>+ 添加记忆</Text>
          </Pressable>
        </View>

        {message ? <Text style={styles.status}>{message}</Text> : null}

        {/* 提取异常提示 (如有) */}
        {extractionJobs.length > 0 ? (
          <View style={styles.extractionBanner}>
            <View style={styles.iconRow}>
              <Feather color={palette.warn} name="alert-triangle" size={15} />
              <Text style={styles.extractionBannerText}>有 {extractionJobs.length} 项提取任务待处理</Text>
            </View>
            <Pressable
              disabled={pending}
              onPress={() => onRefreshExtractionJobs()}
              style={({ pressed }) => buttonFeedback(styles.miniActionBtn, pressed)}
            >
              <Text style={styles.miniActionText}>刷新</Text>
            </Pressable>
          </View>
        ) : null}

        {/* 待审核候选折叠提示 (如有) */}
        {candidates.length > 0 ? (
          <View style={styles.candidateBanner}>
            <Pressable
              onPress={() => setShowCandidates((prev) => !prev)}
              style={({ pressed }) => buttonFeedback(styles.candidateBannerHeader, pressed)}
            >
              <View style={styles.iconRow}>
                <Feather color={palette.brand} name="bell" size={16} />
                <Text style={styles.candidateBannerTitle}>
                  发现 {candidates.length} 条待审核记忆候选
                </Text>
              </View>
              <View style={styles.iconRow}>
                <Text style={styles.candidateBannerToggle}>{showCandidates ? "收起" : "审核"}</Text>
                <Feather color={palette.ink2} name={showCandidates ? "chevron-up" : "chevron-down"} size={15} />
              </View>
            </Pressable>
            {showCandidates ? (
              <View style={styles.candidateList}>
                {candidates.map((candidate) => (
                  <View key={candidate.id} style={styles.candidateCard}>
                    <View style={styles.cardHeader}>
                      <View style={styles.badge}>
                        <Text style={styles.badgeText}>{candidate.tags?.[0] || kindLabel(candidate.kind)}</Text>
                      </View>
                      <Text style={styles.cardDate}>待确认</Text>
                    </View>
                    <Text style={styles.cardBody}>
                      {candidate.title ? `${candidate.title}：` : ""}{candidate.content.goal || "候选内容"}
                    </Text>
                    <View style={styles.candidateActions}>
                      <Pressable
                        onPress={() => onApproveCandidate(candidate)}
                        style={({ pressed }) => buttonFeedback(styles.approveBtn, pressed)}
                      >
                        <Text style={styles.approveBtnText}>通过</Text>
                      </Pressable>
                      {activeMemories.length > 0 ? (
                        <Pressable
                          onPress={() => beginMerge(candidate)}
                          style={({ pressed }) => buttonFeedback(styles.mergeBtn, pressed)}
                        >
                          <Text style={styles.mergeBtnText}>合并</Text>
                        </Pressable>
                      ) : null}
                      <Pressable
                        onPress={() => onRejectCandidate(candidate)}
                        style={({ pressed }) => buttonFeedback(styles.deleteBtn, pressed)}
                      >
                        <Text style={styles.deleteBtnText}>拒绝</Text>
                      </Pressable>
                    </View>
                  </View>
                ))}
              </View>
            ) : null}
          </View>
        ) : null}

        {/* 记忆卡片列表 (Neo-Brutalism 样式) */}
        {activeMemories.map((memory) => {
          const revision = currentRevision(memory);
          const expanded = expandedID === memory.id;
          const bodyText = getMemoryBody(memory, revision);
          const sourceText = getMemorySource(memory, revision);
          const tagText = getCategoryTag(memory);
          const dateText = formatMemoryDate(memory.created_at || revision?.created_at);

          return (
            <Pressable
              key={memory.id}
              onPress={() => setExpandedID(expanded ? "" : memory.id)}
              style={({ pressed }) => buttonFeedback([styles.memoryCard, pressed && styles.cardPressed], pressed)}
            >
              {/* 卡片顶部: 标签徽章 + 时间 */}
              <View style={styles.cardHeader}>
                <View style={styles.badge}>
                  <Text style={styles.badgeText}>{tagText}</Text>
                </View>
                <Text style={styles.cardDate}>{dateText}</Text>
              </View>

              {/* 卡片正文: 粗体直接展示事实 */}
              <Text style={styles.cardBody}>{bodyText}</Text>

              {/* 展开详情: 显示更全面的结构化经验与编辑 */}
              {expanded && revision ? (
                <View style={styles.expandedDetails}>
                  <MemoryContentView content={revision.content} />
                  <View style={styles.expandedMetaRow}>
                    <Text style={styles.cardMeta}>
                      范围: {scopeLabel(memory.scope)} · 频次: {memory.use_count || 0} 次 · 可信度: {Math.round((revision.confidence || 1) * 100)}%
                    </Text>
                    <Pressable
                      onPress={() => beginEdit(memory)}
                      style={({ pressed }) => buttonFeedback(styles.editBtn, pressed)}
                    >
                      <Text style={styles.editBtnText}>编辑</Text>
                    </Pressable>
                  </View>
                </View>
              ) : null}

              {/* 卡片底部: 来源 + [删除] 按钮 */}
              <View style={styles.cardFooter}>
                <Text style={styles.cardSource}>来源: {sourceText}</Text>
                <Pressable
                  onPress={(e) => {
                    e.stopPropagation?.();
                    onDeleteMemory(memory.id);
                  }}
                  style={({ pressed }) => buttonFeedback(styles.deleteBtn, pressed)}
                >
                  <Text style={styles.deleteBtnText}>删除</Text>
                </Pressable>
              </View>
            </Pressable>
          );
        })}

        {!pending && activeMemories.length === 0 ? (
          <View style={styles.emptyContainer}>
            <Text style={styles.emptyText}>暂无沉淀记忆，点击右上角「+ 添加记忆」手动添加或由智能体自动提炼。</Text>
          </View>
        ) : null}

        {/* 底部: 记忆自动化固化 (Dream) 模块 */}
        <View style={styles.dreamCard}>
          <View style={styles.iconRow}>
            <Feather color={palette.brand} name="star" size={16} />
            <Text style={styles.dreamTitle}>记忆自动化固化 (Dream)</Text>
          </View>
          <Text style={styles.dreamSubtitle}>
            Agent 每天凌晨会自动提取高频对话事实并合并至主记忆库
          </Text>
          <Pressable
            disabled={pending}
            onPress={onRunDream}
            style={({ pressed }) => buttonFeedback([styles.dreamBtn, pending && styles.disabled], pressed)}
          >
            <ButtonContent loading={pending} text="立即运行 Dream 整合" />
          </Pressable>
        </View>
      </View>

      {/* 编辑/新建记忆弹窗 */}
      <MemoryEditor
        buttonFeedback={buttonFeedback}
        draft={draft}
        editing={Boolean(editingID)}
        onCancel={() => setShowEditor(false)}
        onChange={setDraft}
        onSubmit={submit}
        pending={pending}
        visible={showEditor}
      />

      {/* 合并候选记忆弹窗 */}
      <MergeMemoryModal
        buttonFeedback={buttonFeedback}
        candidate={mergeCandidate}
        memories={mergeTargets}
        onCancel={() => setMergeCandidate(null)}
        onChangeQuery={setMergeQuery}
        onConfirm={confirmMerge}
        onSelect={setMergeMemoryID}
        pending={pending}
        query={mergeQuery}
        selectedID={mergeMemoryID}
      />
    </>
  );
}

function MergeMemoryModal({
  buttonFeedback,
  candidate,
  memories,
  onCancel,
  onChangeQuery,
  onConfirm,
  onSelect,
  pending,
  query,
  selectedID,
}: {
  buttonFeedback: ButtonFeedback;
  candidate: AIMemoryCandidate | null;
  memories: AIMemory[];
  onCancel: () => void;
  onChangeQuery: (value: string) => void;
  onConfirm: () => void;
  onSelect: (memoryID: string) => void;
  pending: boolean;
  query: string;
  selectedID: string;
}) {
  return (
    <ResponsiveFormModal
      buttonFeedback={buttonFeedback}
      footer={(
        <>
          <Pressable onPress={onCancel} style={({ pressed }) => buttonFeedback(styles.footerCancelButton, pressed)}>
            <Text style={styles.footerCancelText}>取消</Text>
          </Pressable>
          <Pressable
            disabled={pending || !selectedID}
            onPress={onConfirm}
            style={({ pressed }) => buttonFeedback([styles.footerSubmitButton, (pending || !selectedID) && styles.disabled], pressed)}
          >
            <ButtonContent loading={pending} text="确认合并" />
          </Pressable>
        </>
      )}
      onClose={onCancel}
      title="选择要合并的记忆"
      visible={candidate !== null}
    >
      {candidate ? <Text style={styles.mergeHint}>候选：{candidate.title}</Text> : null}
      <TextInput
        onChangeText={onChangeQuery}
        placeholder="搜索标题、目标或标签"
        placeholderTextColor={palette.ink4}
        style={styles.input}
        value={query}
      />
      {memories.map((memory) => {
        const revision = currentRevision(memory);
        const selected = selectedID === memory.id;
        return (
          <Pressable
            key={memory.id}
            onPress={() => onSelect(memory.id)}
            style={({ pressed }) => buttonFeedback([styles.mergeTarget, selected && styles.mergeTargetSelected], pressed)}
          >
            <View style={styles.flex}>
              <Text style={styles.mergeTargetTitle}>{memory.title}</Text>
              <Text numberOfLines={2} style={styles.mergeTargetGoal}>{revision?.content.goal || "无目标描述"}</Text>
              <Text style={styles.cardMeta}>v{memory.current_version} · {(memory.tags || []).map((tag) => `#${tag}`).join(" ") || "无标签"}</Text>
            </View>
            <Feather color={selected ? palette.ok : palette.ink3} name={selected ? "check" : "circle"} size={14} />
          </Pressable>
        );
      })}
      {memories.length === 0 ? <Text style={styles.emptyText}>没有找到可合并的有效记忆。</Text> : null}
    </ResponsiveFormModal>
  );
}

function MemoryEditor({
  buttonFeedback,
  draft,
  editing,
  onCancel,
  onChange,
  onSubmit,
  pending,
  visible,
}: {
  buttonFeedback: ButtonFeedback;
  draft: Draft;
  editing: boolean;
  onCancel: () => void;
  onChange: (draft: Draft) => void;
  onSubmit: () => void;
  pending: boolean;
  visible: boolean;
}) {
  const setContent = (key: keyof AIMemoryInput["content"], value: string) =>
    onChange({ ...draft, content: { ...draft.content, [key]: value } });

  return (
    <ResponsiveFormModal
      buttonFeedback={buttonFeedback}
      footer={(
        <>
          <Pressable onPress={onCancel} style={({ pressed }) => buttonFeedback(styles.footerCancelButton, pressed)}>
            <Text style={styles.footerCancelText}>取消</Text>
          </Pressable>
          <Pressable
            disabled={pending}
            onPress={onSubmit}
            style={({ pressed }) => buttonFeedback([styles.footerSubmitButton, pending && styles.disabled], pressed)}
          >
            <ButtonContent loading={pending} text={editing ? "保存" : "创建记忆"} />
          </Pressable>
        </>
      )}
      onClose={onCancel}
      title={editing ? "编辑记忆" : "新建记忆"}
      visible={visible}
    >
      <TextInput
        autoFocus
        onChangeText={(title) => onChange({ ...draft, title })}
        placeholder="记忆标题或简要描述"
        placeholderTextColor={palette.ink4}
        style={styles.input}
        value={draft.title}
      />
      <View style={styles.segmented}>
        {(["experience", "failure", "decision", "preference"] as AIMemoryKind[]).map((kind) => (
          <Pressable
            key={kind}
            onPress={() => onChange({ ...draft, kind })}
            style={({ pressed }) =>
              buttonFeedback([styles.segment, draft.kind === kind && styles.segmentActive], pressed)
            }
          >
            <Text style={[styles.segmentText, draft.kind === kind && styles.segmentTextActive]}>
              {kindLabel(kind)}
            </Text>
          </Pressable>
        ))}
      </View>
      <EditorField label="主要事实/目标" onChange={(value) => setContent("goal", value)} value={draft.content.goal} />
      <EditorField label="适用条件" onChange={(value) => setContent("applicable_context", value)} value={draft.content.applicable_context || ""} />
      <EditorField label="推荐方案/做法" onChange={(value) => setContent("approach", value)} value={draft.content.approach || ""} />
      <EditorField label="经验结论" onChange={(value) => setContent("lessons", value)} value={draft.content.lessons || ""} />
      <TextInput
        onChangeText={(tagsText) => onChange({ ...draft, tagsText })}
        placeholder="分类标签，用逗号分隔（如：用户习惯, 工程约定）"
        placeholderTextColor={palette.ink4}
        style={styles.input}
        value={draft.tagsText}
      />
    </ResponsiveFormModal>
  );
}

function EditorField({ label, onChange, value }: { label: string; onChange: (value: string) => void; value: string }) {
  return (
    <View style={styles.editorFieldWrapper}>
      <Text style={styles.fieldLabel}>{label}</Text>
      <TextInput
        multiline
        onChangeText={onChange}
        placeholder={label}
        placeholderTextColor={palette.ink4}
        style={[styles.input, styles.textarea]}
        value={value}
      />
    </View>
  );
}

function MemoryContentView({ content }: { content: AIMemoryInput["content"] }) {
  const rows = [
    ["适用条件", content.applicable_context],
    ["方案", content.approach],
    ["结果", content.result],
    ["痛点", content.pain_points],
    ["错误原因", content.root_cause],
    ["经验", content.lessons],
    ["验证", content.verification],
  ];
  const items = rows.filter(([, value]) => Boolean(value));
  if (items.length === 0) return null;

  return (
    <View style={styles.detailGrid}>
      {items.map(([label, value]) => (
        <View key={label} style={styles.detailItem}>
          <Text style={styles.fieldLabel}>{label}</Text>
          <Text style={styles.detailText}>{value}</Text>
        </View>
      ))}
    </View>
  );
}

function currentRevision(memory: AIMemory): AIMemoryRevision | undefined {
  return memory.revisions.find((revision) => revision.version === memory.current_version) || memory.revisions[memory.revisions.length - 1];
}

function getCategoryTag(memory: AIMemory): string {
  if (memory.tags && memory.tags.length > 0 && memory.tags[0].trim()) {
    return memory.tags[0].trim();
  }
  return kindLabel(memory.kind);
}

function formatMemoryDate(dateStr?: string): string {
  if (!dateStr) return "2026-09-18 22:30";
  try {
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return dateStr;
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    const h = String(d.getHours()).padStart(2, "0");
    const min = String(d.getMinutes()).padStart(2, "0");
    return `${y}-${m}-${day} ${h}:${min}`;
  } catch {
    return dateStr;
  }
}

function getMemoryBody(memory: AIMemory, revision?: AIMemoryRevision): string {
  const goal = revision?.content.goal?.trim();
  const title = memory.title?.trim();
  if (goal && title && title !== goal) {
    if (goal.startsWith(title)) return goal;
    return `${title}：${goal}`;
  }
  return goal || title || revision?.content.lessons || "暂无记忆事实描述";
}

function getMemorySource(memory: AIMemory, revision?: AIMemoryRevision): string {
  const src = revision?.sources?.[0];
  if (!src) {
    return revision?.author === "human" ? "User Explicit" : "Chat Session";
  }
  if (src.type === "session" || src.session_id) {
    return `Chat #${src.session_id ? src.session_id.slice(-2) : "42"}`;
  }
  if (src.type === "agent_run" || src.agent_run_id) {
    return `Agent #${src.agent_run_id ? src.agent_run_id.slice(-4) : "Run"}`;
  }
  if (src.type === "manual" || revision?.author === "human") return "User Explicit";
  if (src.type === "dream") return "Dream Consolidation";
  return "User Explicit";
}

function trimContent(content: AIMemoryInput["content"]): AIMemoryInput["content"] {
  return Object.fromEntries(Object.entries(content).map(([key, value]) => [key, value?.trim() || ""])) as AIMemoryInput["content"];
}

function normalizeScope(scope: AIMemoryScope): AIMemoryScope {
  return scope.type === "global" ? { type: "global" } : { type: scope.type, key: scope.key?.trim() || "" };
}

function kindLabel(kind: AIMemoryKind): string {
  return ({ experience: "经验", failure: "失败", decision: "决策", preference: "用户习惯" } as const)[kind] || "记忆";
}

function scopeLabel(scope: AIMemoryScope): string {
  return scope.type === "global" ? "全局" : `${scope.type}:${scope.key || "未指定"}`;
}

const styles = StyleSheet.create({
  content: {
    backgroundColor: palette.canvas,
    gap: 10,
    paddingTop: 10,
    paddingBottom: 40,
  },
  flex: {
    flex: 1,
  },
  iconRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  headerRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    marginBottom: 4,
  },
  headerTitle: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.bold,
  },
  addBtn: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 12,
    paddingVertical: 5,
  },
  addBtnText: {
    color: palette.inkInverse,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  status: {
    backgroundColor: palette.okWash,
    borderColor: palette.okBorder,
    borderRadius: radius.sm,
    borderWidth: 1,
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
    padding: 8,
  },
  extractionBanner: {
    alignItems: "center",
    backgroundColor: palette.errWash,
    borderColor: palette.errBorder,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    padding: 10,
  },
  extractionBannerText: {
    color: palette.err,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  miniActionBtn: {
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  miniActionText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  candidateBanner: {
    backgroundColor: palette.brandWash,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    overflow: "hidden",
  },
  candidateBannerHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  candidateBannerTitle: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  candidateBannerToggle: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  candidateList: {
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    gap: 8,
    padding: 10,
  },
  candidateCard: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    padding: 10,
  },
  candidateActions: {
    flexDirection: "row",
    gap: 8,
    justifyContent: "flex-end",
    marginTop: 8,
  },
  approveBtn: {
    alignItems: "center",
    backgroundColor: palette.okWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    paddingHorizontal: 10,
    paddingVertical: 4,
  },
  approveBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  mergeBtn: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    paddingHorizontal: 10,
    paddingVertical: 4,
  },
  mergeBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },

  /* Neo-Brutalism 记忆卡片样式 */
  memoryCard: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairlineStrong,
    borderLeftColor: palette.brandDeep,
    borderLeftWidth: 1,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 6,
    padding: 12,
    ...shadow.card,
  },
  cardPressed: {
    opacity: 0.92,
    transform: [{ translateY: 1 }],
  },
  cardHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  badge: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.xs,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  badgeText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  cardDate: {
    color: palette.ink3,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  cardBody: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
    lineHeight: 19,
    marginVertical: 2,
  },
  cardFooter: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    marginTop: 2,
  },
  cardSource: {
    color: palette.ink3,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  deleteBtn: {
    alignItems: "center",
    backgroundColor: palette.errWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    paddingHorizontal: 10,
    paddingVertical: 3,
  },
  deleteBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  expandedDetails: {
    borderTopColor: palette.hairline,
    borderTopWidth: 1,
    gap: 8,
    marginTop: 6,
    paddingTop: 8,
  },
  expandedMetaRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    marginTop: 4,
  },
  cardMeta: {
    color: palette.ink3,
    fontSize: font.caption,
  },
  editBtn: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    paddingHorizontal: 10,
    paddingVertical: 3,
  },
  editBtnText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },

  /* 底部 Dream 卡片 (虚线边框) */
  dreamCard: {
    alignItems: "center",
    backgroundColor: palette.brandWash,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.md,
    borderStyle: "dashed",
    borderWidth: 1,
    gap: 6,
    marginTop: 6,
    padding: 16,
  },
  dreamTitle: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
    textAlign: "center",
  },
  dreamSubtitle: {
    color: palette.ink2,
    fontSize: font.caption,
    lineHeight: 16,
    textAlign: "center",
  },
  dreamBtn: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    marginTop: 4,
    minHeight: 38,
    paddingHorizontal: 18,
    paddingVertical: 8,
  },

  /* 通用表单与弹窗样式 */
  emptyContainer: {
    alignItems: "center",
    paddingVertical: 32,
  },
  emptyText: {
    color: palette.ink3,
    fontSize: font.label,
    textAlign: "center",
  },
  disabled: {
    opacity: 0.5,
  },
  input: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    color: palette.ink1,
    fontSize: font.body,
    minHeight: 40,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  textarea: {
    minHeight: 70,
    textAlignVertical: "top",
  },
  editorFieldWrapper: {
    gap: 4,
  },
  fieldLabel: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  detailGrid: {
    gap: 6,
  },
  detailItem: {
    gap: 2,
  },
  detailText: {
    color: palette.ink1,
    fontSize: font.label,
    lineHeight: 17,
  },
  segmented: {
    backgroundColor: palette.surface3,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.pill,
    borderWidth: 1,
    flexDirection: "row",
    gap: 2,
    padding: 2,
  },
  segment: {
    alignItems: "center",
    borderRadius: radius.pill,
    flex: 1,
    justifyContent: "center",
    minHeight: 30,
    paddingHorizontal: 6,
  },
  segmentActive: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairlineStrong,
    borderWidth: 1,
  },
  segmentText: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  segmentTextActive: {
    color: palette.ink1,
    fontWeight: weight.bold,
  },
  footerCancelButton: {
    alignItems: "center",
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 38,
    minWidth: 80,
    paddingHorizontal: 12,
  },
  footerCancelText: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  footerSubmitButton: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 38,
    minWidth: 100,
    paddingHorizontal: 14,
  },
  mergeHint: {
    backgroundColor: palette.okWash,
    borderRadius: radius.sm,
    color: palette.ok,
    fontSize: font.label,
    lineHeight: 18,
    padding: 8,
  },
  mergeTarget: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  mergeTargetSelected: {
    backgroundColor: palette.okWash,
    borderColor: palette.okBorder,
    borderWidth: 1,
  },
  mergeTargetTitle: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  mergeTargetGoal: {
    color: palette.ink1,
    fontSize: font.label,
    lineHeight: 16,
    marginTop: 2,
  },
  mergeCheck: {
    color: palette.ink3,
    fontSize: font.title,
    width: 22,
  },
  mergeCheckSelected: {
    color: palette.ok,
    fontWeight: weight.bold,
  },
});
