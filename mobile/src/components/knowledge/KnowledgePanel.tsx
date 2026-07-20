import { useMemo, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import type {
  KnowledgeBase,
  KnowledgeCategory,
  KnowledgeDocument,
  KnowledgeIndexProfile,
  KnowledgeIndexingJob,
  KnowledgeSearchPreviewResultPayload,
  RAGSettings,
  SessionSummary,
} from "../../protocol";
import type { KnowledgeBaseChanges } from "../../types/knowledge";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  activeSession?: SessionSummary;
  buttonFeedback: ButtonFeedback;
  categories: KnowledgeCategory[];
  documents: KnowledgeDocument[];
  jobs: KnowledgeIndexingJob[];
  knowledgeBases: KnowledgeBase[];
  message: string;
  onCreateCategory: (name: string, parentID?: string) => boolean;
  onCreateKnowledgeBase: (name: string, categoryID?: string, profileID?: string) => boolean;
  onDeleteCategory: (categoryID: string, recursive?: boolean) => boolean;
  onDeleteDocument: (knowledgeBaseID: string, documentID: string) => void;
  onDeleteKnowledgeBase: (knowledgeBaseID: string) => boolean;
  onMoveCategory: (categoryID: string, parentID?: string) => boolean;
  onRefresh: () => void;
  onRefreshDocuments: () => void;
  onRetryDocument: (knowledgeBaseID: string, jobID: string) => void;
  onSearch: (query: string, settings?: RAGSettings) => boolean;
  onSelectKnowledgeBase: (knowledgeBaseID: string) => void;
  onSetRAG: (settings: RAGSettings) => boolean;
  onUpdateKnowledgeBase: (base: KnowledgeBase, changes: KnowledgeBaseChanges) => boolean;
  onUploadDocument: () => void;
  profiles: KnowledgeIndexProfile[];
  searchResult: KnowledgeSearchPreviewResultPayload | null;
};

type DetailTab = "documents" | "search" | "config";

export function KnowledgePanel({
  activeSession,
  buttonFeedback,
  categories,
  documents,
  jobs,
  knowledgeBases,
  message,
  onCreateCategory,
  onCreateKnowledgeBase,
  onDeleteCategory,
  onDeleteDocument,
  onDeleteKnowledgeBase,
  onMoveCategory,
  onRefresh,
  onRefreshDocuments,
  onRetryDocument,
  onSearch,
  onSelectKnowledgeBase,
  onSetRAG,
  onUpdateKnowledgeBase,
  onUploadDocument,
  profiles,
  searchResult,
}: Props) {
  const [selectedID, setSelectedID] = useState("");
  const [tab, setTab] = useState<DetailTab>("documents");
  const [query, setQuery] = useState("");
  const [categoryName, setCategoryName] = useState("");
  const [baseName, setBaseName] = useState("");
  const [parentID, setParentID] = useState("");
  const [profileID, setProfileID] = useState("");
  const [managedCategoryID, setManagedCategoryID] = useState("");
  const [categoryMoveParentID, setCategoryMoveParentID] = useState("");
  const [recursiveDeleteID, setRecursiveDeleteID] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const settings = normalizeRAG(activeSession?.rag);
  const selectedBase = knowledgeBases.find((base) => base.id === selectedID);
  const managedCategory = categories.find((category) => category.id === managedCategoryID);
  const availableMoveParents = managedCategory
    ? categories.filter((category) => category.id !== managedCategory.id && !(category.ancestor_ids || []).includes(managedCategory.id))
    : [];
  const sortedBases = useMemo(() => [...knowledgeBases].sort((left, right) => left.name.localeCompare(right.name)), [knowledgeBases]);

  const toggleID = (values: string[], value: string) => values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
  const updateRAG = (next: Partial<RAGSettings>) => onSetRAG({ ...settings, ...next });

  return (
    <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <View style={styles.headerRow}>
        <View style={styles.flex}>
          <Text style={styles.eyebrow}>KNOWLEDGE</Text>
          <Text style={styles.title}>{selectedBase ? selectedBase.name : "知识库"}</Text>
        </View>
        <Pressable onPress={onRefresh} style={({ pressed }) => buttonFeedback(styles.actionButton, pressed)}>
          <Text style={styles.actionText}>刷新</Text>
        </Pressable>
      </View>

      <View style={styles.section}>
        <Text style={styles.sectionTitle}>当前会话</Text>
        <View style={styles.modeRow}>
          {(["off", "manual", "auto", "always"] as const).map((mode) => (
            <Pressable key={mode} onPress={() => updateRAG({ mode })} style={({ pressed }) => buttonFeedback([styles.modeButton, settings.mode === mode && styles.modeButtonActive], pressed)}>
              <Text style={[styles.modeText, settings.mode === mode && styles.modeTextActive]}>{modeLabel(mode)}</Text>
            </Pressable>
          ))}
        </View>
        <View style={styles.rowBetween}>
          <Text style={styles.meta}>Top K</Text>
          <TextInput keyboardType="number-pad" onChangeText={(value) => updateRAG({ top_k: Math.max(1, Number(value) || 8) })} style={styles.smallInput} value={String(settings.top_k)} />
        </View>
        <Text style={styles.meta}>范围</Text>
        <View style={styles.chipWrap}>
          {categories.map((category) => (
            <Pressable key={category.id} onPress={() => updateRAG({ category_ids: toggleID(settings.category_ids, category.id) })} style={({ pressed }) => buttonFeedback([styles.scopeChip, settings.category_ids.includes(category.id) && styles.scopeChipActive], pressed)}>
              <Text style={styles.scopeText}>{settings.category_ids.includes(category.id) ? "✓ " : ""}{categoryPathLabel(category, categories)}</Text>
            </Pressable>
          ))}
          {sortedBases.map((base) => (
            <Pressable key={base.id} onPress={() => updateRAG({ knowledge_base_ids: toggleID(settings.knowledge_base_ids, base.id) })} style={({ pressed }) => buttonFeedback([styles.scopeChip, settings.knowledge_base_ids.includes(base.id) && styles.scopeChipActive], pressed)}>
              <Text style={styles.scopeText}>{settings.knowledge_base_ids.includes(base.id) ? "✓ " : ""}{base.name}</Text>
            </Pressable>
          ))}
        </View>
      </View>

      {selectedBase ? (
        <View style={styles.section}>
          <View style={styles.detailHeader}>
            <Pressable onPress={() => setSelectedID("")} style={({ pressed }) => buttonFeedback(styles.backButton, pressed)}><Text style={styles.actionText}>返回</Text></Pressable>
            <View style={styles.flex}><Text style={styles.sectionTitle}>{selectedBase.name}</Text><Text style={styles.meta}>{selectedBase.description || "未填写描述"}</Text></View>
          </View>
          <View style={styles.tabRow}>
            {(["documents", "search", "config"] as const).map((value) => <Pressable key={value} onPress={() => setTab(value)} style={({ pressed }) => buttonFeedback([styles.tabButton, tab === value && styles.tabButtonActive], pressed)}><Text style={styles.tabText}>{tabLabel(value)}</Text></Pressable>)}
          </View>
          {tab === "documents" ? <DocumentTab buttonFeedback={buttonFeedback} documents={documents} jobs={jobs} knowledgeBaseID={selectedBase.id} onDelete={onDeleteDocument} onRefresh={onRefreshDocuments} onRetry={onRetryDocument} onUpload={onUploadDocument} /> : null}
          {tab === "search" ? <SearchTab buttonFeedback={buttonFeedback} query={query} result={searchResult} settings={settings} onChangeQuery={setQuery} onSearch={() => onSearch(query, { ...settings, knowledge_base_ids: [selectedBase.id], category_ids: [] })} /> : null}
          {tab === "config" ? <ConfigTab base={selectedBase} buttonFeedback={buttonFeedback} categories={categories} onDelete={onDeleteKnowledgeBase} onUpdate={onUpdateKnowledgeBase} profiles={profiles} /> : null}
        </View>
      ) : (
        <View style={styles.section}>
          <View style={styles.sectionHeader}><Text style={styles.sectionTitle}>分类与知识库</Text><Text style={styles.meta}>{knowledgeBases.length} 个知识库</Text></View>
          <CategoryTree
            bases={sortedBases}
            buttonFeedback={buttonFeedback}
            categories={categories}
            expanded={expanded}
            onManage={(category) => {
              setManagedCategoryID(category.id);
              setCategoryMoveParentID(category.parent_id || "");
              setRecursiveDeleteID("");
            }}
            onSelect={(id) => { setSelectedID(id); onSelectKnowledgeBase(id); }}
            onToggle={(id) => setExpanded((current) => ({ ...current, [id]: !current[id] }))}
          />
          {managedCategory ? (
            <View style={styles.managementPanel}>
              <View style={styles.sectionHeader}>
                <View style={styles.flex}>
                  <Text style={styles.sectionTitle}>管理分类</Text>
                  <Text style={styles.meta}>{categoryPathLabel(managedCategory, categories)}</Text>
                </View>
                <Pressable onPress={() => setManagedCategoryID("")} style={({ pressed }) => buttonFeedback(styles.backButton, pressed)}><Text style={styles.actionText}>完成</Text></Pressable>
              </View>
              <Text style={styles.meta}>移动到</Text>
              <View style={styles.chipWrap}>
                <Pressable onPress={() => setCategoryMoveParentID("")} style={[styles.scopeChip, !categoryMoveParentID && styles.scopeChipActive]}><Text style={styles.scopeText}>根目录</Text></Pressable>
                {availableMoveParents.map((category) => <Pressable key={category.id} onPress={() => setCategoryMoveParentID(category.id)} style={[styles.scopeChip, categoryMoveParentID === category.id && styles.scopeChipActive]}><Text style={styles.scopeText}>{categoryPathLabel(category, categories)}</Text></Pressable>)}
              </View>
              <View style={styles.inlineActions}>
                <Pressable disabled={categoryMoveParentID === (managedCategory.parent_id || "")} onPress={() => onMoveCategory(managedCategory.id, categoryMoveParentID)} style={({ pressed }) => buttonFeedback([styles.secondaryButton, categoryMoveParentID === (managedCategory.parent_id || "") && styles.disabledButton], pressed)}><Text style={styles.secondaryText}>移动分类</Text></Pressable>
                <Pressable onPress={() => onDeleteCategory(managedCategory.id, false)} style={({ pressed }) => buttonFeedback(styles.deleteOutlineButton, pressed)}><Text style={styles.deleteText}>删除空分类</Text></Pressable>
              </View>
              <Pressable
                onPress={() => {
                  if (recursiveDeleteID !== managedCategory.id) {
                    setRecursiveDeleteID(managedCategory.id);
                    return;
                  }
                  if (onDeleteCategory(managedCategory.id, true)) setRecursiveDeleteID("");
                }}
                style={({ pressed }) => buttonFeedback(styles.dangerButton, pressed)}
              >
                <Text style={styles.dangerText}>{recursiveDeleteID === managedCategory.id ? "再次确认递归删除" : "递归删除分类及知识库"}</Text>
              </Pressable>
            </View>
          ) : null}
          <Text style={styles.meta}>创建位置</Text>
          <View style={styles.chipWrap}>
            <Pressable onPress={() => setParentID("")} style={[styles.scopeChip, !parentID && styles.scopeChipActive]}><Text style={styles.scopeText}>根目录</Text></Pressable>
            {categories.map((category) => <Pressable key={category.id} onPress={() => setParentID(category.id)} style={[styles.scopeChip, parentID === category.id && styles.scopeChipActive]}><Text style={styles.scopeText}>{categoryPathLabel(category, categories)}</Text></Pressable>)}
          </View>
          <View style={styles.createRow}>
            <TextInput onChangeText={setCategoryName} placeholder="新分类" placeholderTextColor="#776f66" style={[styles.input, styles.flex]} value={categoryName} />
            <Pressable onPress={() => { if (onCreateCategory(categoryName, parentID)) setCategoryName(""); }} style={({ pressed }) => buttonFeedback(styles.actionButton, pressed)}><Text style={styles.actionText}>添加分类</Text></Pressable>
          </View>
          {profiles.length > 0 ? <><Text style={styles.meta}>IndexProfile</Text><View style={styles.chipWrap}>{profiles.filter((profile) => profile.status === "active").map((profile) => <Pressable key={profile.id} onPress={() => setProfileID(profile.id)} style={[styles.scopeChip, profileID === profile.id && styles.scopeChipActive]}><Text style={styles.scopeText}>{profile.name}</Text></Pressable>)}</View></> : null}
          <View style={styles.createRow}>
            <TextInput onChangeText={setBaseName} placeholder="新知识库" placeholderTextColor="#776f66" style={[styles.input, styles.flex]} value={baseName} />
            <Pressable onPress={() => { if (onCreateKnowledgeBase(baseName, parentID, profileID)) setBaseName(""); }} style={({ pressed }) => buttonFeedback(styles.actionButton, pressed)}><Text style={styles.actionText}>添加知识库</Text></Pressable>
          </View>
          {message ? <Text style={styles.message}>{message}</Text> : null}
        </View>
      )}
    </ScrollView>
  );
}

function CategoryTree({ buttonFeedback, categories, bases, expanded, onManage, onToggle, onSelect }: { buttonFeedback: ButtonFeedback; categories: KnowledgeCategory[]; bases: KnowledgeBase[]; expanded: Record<string, boolean>; onManage: (category: KnowledgeCategory) => void; onToggle: (id: string) => void; onSelect: (id: string) => void }) {
  const roots = categories.filter((category) => !category.parent_id);
  const renderCategory = (category: KnowledgeCategory, depth: number) => {
    const children = categories.filter((item) => item.parent_id === category.id);
    const categoryBases = bases.filter((base) => base.category_id === category.id);
    const isOpen = expanded[category.id] !== false;
    return (
      <View key={category.id}>
        <View style={[styles.treeRow, { paddingLeft: 10 + depth * 18 }]}>
          <Pressable onPress={() => onToggle(category.id)} style={({ pressed }) => buttonFeedback(styles.treeMain, pressed)}>
            <Text style={styles.treeArrow}>{isOpen ? "▾" : "▸"}</Text>
            <Text numberOfLines={1} style={styles.treeName}>{category.name}</Text>
            <Text style={styles.treeCount}>{categoryBases.length}</Text>
          </Pressable>
          <Pressable onPress={() => onManage(category)} style={({ pressed }) => buttonFeedback(styles.manageButton, pressed)}><Text style={styles.manageText}>管理</Text></Pressable>
        </View>
        {isOpen ? <>{categoryBases.map((base) => <Pressable key={base.id} onPress={() => onSelect(base.id)} style={({ pressed }) => buttonFeedback([styles.baseRow, { paddingLeft: 34 + depth * 18 }], pressed)}><Text style={styles.baseDot}>●</Text><Text numberOfLines={1} style={styles.flex}>{base.name}</Text><Text style={styles.statusText}>{base.rag_enabled ? "RAG" : "OFF"}</Text></Pressable>)}{children.map((child) => renderCategory(child, depth + 1))}</> : null}
      </View>
    );
  };
  return <View>{roots.map((category) => renderCategory(category, 0))}{bases.filter((base) => !base.category_id).map((base) => <Pressable key={base.id} onPress={() => onSelect(base.id)} style={({ pressed }) => buttonFeedback(styles.baseRow, pressed)}><Text style={styles.baseDot}>●</Text><Text style={styles.flex}>{base.name}</Text><Text style={styles.statusText}>{base.rag_enabled ? "RAG" : "OFF"}</Text></Pressable>)}</View>;
}

function DocumentTab({ buttonFeedback, documents, jobs, knowledgeBaseID, onDelete, onRefresh, onRetry, onUpload }: { buttonFeedback: ButtonFeedback; documents: KnowledgeDocument[]; jobs: KnowledgeIndexingJob[]; knowledgeBaseID: string; onDelete: (baseID: string, documentID: string) => void; onRefresh: () => void; onRetry: (baseID: string, jobID: string) => void; onUpload: () => void }) {
  return <View><View style={styles.rowBetween}><Text style={styles.meta}>{documents.length} 个文档</Text><View style={styles.inlineActions}><Pressable onPress={onRefresh} style={({ pressed }) => buttonFeedback(styles.secondaryButton, pressed)}><Text style={styles.secondaryText}>刷新</Text></Pressable><Pressable onPress={onUpload} style={({ pressed }) => buttonFeedback(styles.actionButton, pressed)}><Text style={styles.actionText}>上传</Text></Pressable></View></View>{documents.map((document) => { const job = jobs.find((item) => item.document_id === document.id); const progress = job && job.total_chunks > 0 ? Math.round((job.completed_chunks / job.total_chunks) * 100) : 0; return <View key={document.id} style={styles.documentRow}><View style={styles.flex}><Text style={styles.documentName}>{document.file_name}</Text><Text style={styles.meta}>{document.status}{job ? ` · ${job.stage} ${progress}%` : ""}</Text>{document.failure_reason ? <Text style={styles.errorText}>{document.failure_reason}</Text> : null}</View>{document.status === "failed" && job ? <Pressable onPress={() => onRetry(knowledgeBaseID, job.id)} style={({ pressed }) => buttonFeedback(styles.secondaryButton, pressed)}><Text style={styles.secondaryText}>重试</Text></Pressable> : null}<Pressable onPress={() => onDelete(knowledgeBaseID, document.id)} style={({ pressed }) => buttonFeedback(styles.deleteButton, pressed)}><Text style={styles.deleteText}>删除</Text></Pressable></View>; })}{documents.length === 0 ? <Text style={styles.empty}>暂无文档</Text> : null}</View>;
}

function SearchTab({ buttonFeedback, query, result, settings, onChangeQuery, onSearch }: { buttonFeedback: ButtonFeedback; query: string; result: KnowledgeSearchPreviewResultPayload | null; settings: RAGSettings; onChangeQuery: (value: string) => void; onSearch: () => void }) {
  return <View><TextInput onChangeText={onChangeQuery} onSubmitEditing={onSearch} placeholder="输入检索问题" placeholderTextColor="#776f66" style={styles.input} value={query} /><Pressable onPress={onSearch} style={({ pressed }) => buttonFeedback(styles.actionButton, pressed)}><Text style={styles.actionText}>检索</Text></Pressable>{result ? <View style={styles.searchResult}><Text style={styles.meta}>{result.hits.length} 个片段 · {result.profiles.length} 个 Profile</Text>{result.error ? <Text style={styles.errorText}>{result.error}</Text> : null}{result.hits.map((hit) => <View key={hit.chunk_id} style={styles.hitRow}><Text style={styles.hitTitle}>[{hit.rank}] {hit.source_name || hit.document_id} · {hit.origin}</Text><Text selectable style={styles.hitText}>{hit.text}</Text></View>)}</View> : null}</View>;
}

function ConfigTab({ base, buttonFeedback, categories, onDelete, onUpdate, profiles }: { base: KnowledgeBase; buttonFeedback: ButtonFeedback; categories: KnowledgeCategory[]; onDelete: (knowledgeBaseID: string) => boolean; onUpdate: (base: KnowledgeBase, changes: KnowledgeBaseChanges) => boolean; profiles: KnowledgeIndexProfile[] }) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const activeProfiles = profiles.filter((profile) => profile.status === "active" && !profile.deleted);
  return (
    <View style={styles.configContent}>
      <Text style={styles.meta}>所属分类</Text>
      <View style={styles.chipWrap}>
        <Pressable onPress={() => onUpdate(base, { categoryID: "" })} style={[styles.scopeChip, !base.category_id && styles.scopeChipActive]}><Text style={styles.scopeText}>根目录</Text></Pressable>
        {categories.map((category) => <Pressable key={category.id} onPress={() => onUpdate(base, { categoryID: category.id })} style={[styles.scopeChip, base.category_id === category.id && styles.scopeChipActive]}><Text style={styles.scopeText}>{categoryPathLabel(category, categories)}</Text></Pressable>)}
      </View>
      <View style={styles.rowBetween}>
        <View><Text style={styles.meta}>RAG 状态</Text><Text style={styles.documentName}>{base.rag_enabled ? "已启用" : "未启用"}</Text></View>
        <Pressable
          disabled={!base.rag_enabled && !base.active_index_profile_id}
          onPress={() => onUpdate(base, { ragEnabled: !base.rag_enabled })}
          style={({ pressed }) => buttonFeedback([styles.secondaryButton, !base.rag_enabled && !base.active_index_profile_id && styles.disabledButton], pressed)}
        >
          <Text style={styles.secondaryText}>{base.rag_enabled ? "关闭 RAG" : "启用 RAG"}</Text>
        </Pressable>
      </View>
      <Text style={styles.meta}>Active IndexProfile</Text>
      <View style={styles.chipWrap}>
        {activeProfiles.map((profile) => <Pressable key={profile.id} onPress={() => onUpdate(base, { activeIndexProfileID: profile.id, ragEnabled: true })} style={[styles.scopeChip, base.active_index_profile_id === profile.id && styles.scopeChipActive]}><Text style={styles.scopeText}>{profile.name}</Text></Pressable>)}
      </View>
      <Pressable
        onPress={() => {
          if (!confirmDelete) {
            setConfirmDelete(true);
            return;
          }
          if (onDelete(base.id)) setConfirmDelete(false);
        }}
        style={({ pressed }) => buttonFeedback(styles.dangerButton, pressed)}
      >
        <Text style={styles.dangerText}>{confirmDelete ? "再次确认删除知识库" : "删除知识库"}</Text>
      </Pressable>
    </View>
  );
}

function normalizeRAG(value?: RAGSettings): RAGSettings { return { mode: value?.mode || "auto", knowledge_base_ids: value?.knowledge_base_ids || [], category_ids: value?.category_ids || [], top_k: value?.top_k || 8 }; }
function modeLabel(mode: string) { return ({ off: "关闭", manual: "手动", auto: "自动", always: "每轮" } as Record<string, string>)[mode] || mode; }
function tabLabel(tab: DetailTab) { return ({ documents: "文档", search: "检索测试", config: "配置" })[tab]; }
function categoryPathLabel(category: KnowledgeCategory, categories: KnowledgeCategory[]) {
  const names = (category.ancestor_ids || []).map((id) => categories.find((candidate) => candidate.id === id)?.name).filter(Boolean);
  return [...names, category.name].join(" / ");
}

const styles = StyleSheet.create({
  content: { gap: 12, padding: 14, paddingBottom: 28 },
  headerRow: { alignItems: "center", flexDirection: "row", gap: 10 },
  detailHeader: { alignItems: "center", flexDirection: "row", gap: 10, marginBottom: 10 },
  section: { backgroundColor: "#fffaf0", borderColor: "#12100e", borderRadius: 8, borderWidth: 2, gap: 10, padding: 12 },
  sectionHeader: { alignItems: "center", flexDirection: "row", justifyContent: "space-between" },
  sectionTitle: { color: "#12100e", fontSize: 16, fontWeight: "900" },
  eyebrow: { color: "#6c665f", fontSize: 11, fontWeight: "900" },
  title: { color: "#12100e", fontSize: 24, fontWeight: "900" },
  meta: { color: "#6c665f", fontSize: 12, fontWeight: "700" },
  flex: { flex: 1, minWidth: 0 },
  rowBetween: { alignItems: "center", flexDirection: "row", justifyContent: "space-between", gap: 8 },
  inlineActions: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 8 },
  modeRow: { flexDirection: "row", gap: 6 },
  modeButton: { borderColor: "#b8aea1", borderRadius: 6, borderWidth: 2, paddingHorizontal: 10, paddingVertical: 8 },
  modeButtonActive: { backgroundColor: "#ffd84f", borderColor: "#12100e" },
  modeText: { color: "#6c665f", fontSize: 12, fontWeight: "900" },
  modeTextActive: { color: "#12100e" },
  actionButton: { alignItems: "center", backgroundColor: "#ffd84f", borderColor: "#12100e", borderRadius: 6, borderWidth: 2, paddingHorizontal: 10, paddingVertical: 8 },
  actionText: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  secondaryButton: { alignItems: "center", backgroundColor: "#4fd7ee", borderColor: "#12100e", borderRadius: 6, borderWidth: 2, paddingHorizontal: 9, paddingVertical: 7 },
  secondaryText: { color: "#12100e", fontSize: 11, fontWeight: "900" },
  disabledButton: { opacity: 0.4 },
  backButton: { backgroundColor: "#efe4d2", borderColor: "#12100e", borderRadius: 6, borderWidth: 2, paddingHorizontal: 9, paddingVertical: 7 },
  deleteOutlineButton: { alignItems: "center", borderColor: "#b5412b", borderRadius: 6, borderWidth: 1, paddingHorizontal: 9, paddingVertical: 7 },
  dangerButton: { alignItems: "center", backgroundColor: "#ef7868", borderColor: "#12100e", borderRadius: 6, borderWidth: 2, paddingHorizontal: 10, paddingVertical: 9 },
  dangerText: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  smallInput: { borderColor: "#b8aea1", borderRadius: 5, borderWidth: 1, color: "#12100e", minWidth: 58, paddingHorizontal: 8, paddingVertical: 5, textAlign: "right" },
  input: { borderColor: "#b8aea1", borderRadius: 6, borderWidth: 1, color: "#12100e", minHeight: 40, paddingHorizontal: 10, paddingVertical: 8 },
  chipWrap: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
  scopeChip: { borderColor: "#b8aea1", borderRadius: 6, borderWidth: 1, paddingHorizontal: 9, paddingVertical: 7 },
  scopeChipActive: { backgroundColor: "#b9e9b0", borderColor: "#12100e" },
  scopeText: { color: "#12100e", fontSize: 12, fontWeight: "800" },
  treeRow: { alignItems: "center", flexDirection: "row", gap: 7, minHeight: 38 },
  treeMain: { alignItems: "center", flex: 1, flexDirection: "row", gap: 7, minHeight: 38, minWidth: 0 },
  treeArrow: { color: "#6c665f", fontSize: 15, width: 14 },
  treeName: { color: "#12100e", flex: 1, fontSize: 14, fontWeight: "900" },
  treeCount: { color: "#6c665f", fontSize: 11, fontWeight: "800" },
  manageButton: { paddingHorizontal: 7, paddingVertical: 6 },
  manageText: { color: "#6c665f", fontSize: 11, fontWeight: "900" },
  baseRow: { alignItems: "center", flexDirection: "row", gap: 8, minHeight: 36, paddingHorizontal: 10 },
  baseDot: { color: "#4fd7ee", fontSize: 10 },
  statusText: { color: "#6c665f", fontSize: 10, fontWeight: "900" },
  createRow: { alignItems: "center", flexDirection: "row", gap: 7 },
  managementPanel: { borderBottomColor: "#ded2c3", borderBottomWidth: 1, borderTopColor: "#ded2c3", borderTopWidth: 1, gap: 10, paddingVertical: 12 },
  message: { color: "#26743b", fontSize: 12, fontWeight: "800" },
  tabRow: { flexDirection: "row", gap: 6 },
  tabButton: { borderBottomColor: "#b8aea1", borderBottomWidth: 2, paddingHorizontal: 8, paddingVertical: 7 },
  tabButtonActive: { borderBottomColor: "#12100e" },
  tabText: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  configContent: { gap: 10 },
  documentRow: { alignItems: "center", borderTopColor: "#ded2c3", borderTopWidth: 1, flexDirection: "row", gap: 8, paddingVertical: 10 },
  documentName: { color: "#12100e", fontSize: 14, fontWeight: "900" },
  deleteButton: { paddingHorizontal: 5, paddingVertical: 5 },
  deleteText: { color: "#b5412b", fontSize: 11, fontWeight: "900" },
  errorText: { color: "#b5412b", fontSize: 11, fontWeight: "700" },
  empty: { color: "#6c665f", paddingVertical: 12, textAlign: "center" },
  searchResult: { gap: 8, marginTop: 10 },
  hitRow: { borderTopColor: "#ded2c3", borderTopWidth: 1, gap: 4, paddingTop: 10 },
  hitTitle: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  hitText: { color: "#3d3833", fontSize: 12, lineHeight: 18 },
});
