import { useEffect, useMemo, useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

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
import { ButtonContent } from "../common/ButtonContent";
import { ResponsiveFormModal } from "../common/ResponsiveFormModal";

export type KnowledgePanelProps = {
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
  pendingKnowledge: boolean;
  pendingSettings: boolean;
  profiles: KnowledgeIndexProfile[];
  searchResult: KnowledgeSearchPreviewResultPayload | null;
};

type DetailTab = "search" | "config";

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
  pendingKnowledge,
  pendingSettings,
  profiles,
  searchResult,
}: KnowledgePanelProps) {
  const [selectedID, setSelectedID] = useState("");
  const [detailTab, setDetailTab] = useState<DetailTab | null>(null);
  const [filterText, setFilterText] = useState("");
  const [semanticQuery, setSemanticQuery] = useState("");
  const [showCatalogManagement, setShowCatalogManagement] = useState(false);
  const [showSessionRAG, setShowSessionRAG] = useState(false);
  const [categoryName, setCategoryName] = useState("");
  const [baseName, setBaseName] = useState("");
  const [parentID, setParentID] = useState("");
  const [profileID, setProfileID] = useState("");
  const [managedCategoryID, setManagedCategoryID] = useState("");
  const [categoryMoveParentID, setCategoryMoveParentID] = useState("");
  const [recursiveDeleteID, setRecursiveDeleteID] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [pendingRAGMode, setPendingRAGMode] = useState<string | null>(null);

  const sortedBases = useMemo(
    () => [...knowledgeBases].sort((left, right) => left.name.localeCompare(right.name)),
    [knowledgeBases],
  );
  const selectedBase = sortedBases.find((base) => base.id === selectedID);
  const managedCategory = categories.find((category) => category.id === managedCategoryID);
  const availableMoveParents = managedCategory
    ? categories.filter(
        (category) =>
          category.id !== managedCategory.id &&
          !(category.ancestor_ids || []).includes(managedCategory.id),
      )
    : [];
  const settings = normalizeRAG(activeSession?.rag);
  const [topKDraft, setTopKDraft] = useState(String(settings.top_k));
  const canUpdateRAG = Boolean(activeSession);

  useEffect(() => {
    if (selectedID && sortedBases.some((base) => base.id === selectedID)) return;
    const nextID = sortedBases[0]?.id || "";
    setSelectedID(nextID);
    if (nextID) onSelectKnowledgeBase(nextID);
  }, [onSelectKnowledgeBase, selectedID, sortedBases]);

  useEffect(() => {
    if (!pendingSettings) {
      setPendingRAGMode(null);
      setTopKDraft(String(settings.top_k));
    }
  }, [pendingSettings, settings.top_k]);

  const selectBase = (knowledgeBaseID: string) => {
    setSelectedID(knowledgeBaseID);
    setDetailTab(null);
    onSelectKnowledgeBase(knowledgeBaseID);
  };

  const openBaseSettings = (knowledgeBaseID: string) => {
    setSelectedID(knowledgeBaseID);
    setDetailTab("config");
    onSelectKnowledgeBase(knowledgeBaseID);
  };

  const toggleID = (values: string[], value: string) =>
    values.includes(value) ? values.filter((item) => item !== value) : [...values, value];

  const updateRAG = (next: Partial<RAGSettings>) => {
    if (!canUpdateRAG || pendingSettings) return false;
    const sent = onSetRAG({ ...settings, ...next });
    if (sent && next.mode) setPendingRAGMode(next.mode);
    return sent;
  };

  const commitTopK = () => {
    const topK = Math.max(1, Number(topKDraft) || 8);
    setTopKDraft(String(topK));
    if (topK !== settings.top_k) updateRAG({ top_k: topK });
  };

  const closeCatalogManagement = () => {
    setManagedCategoryID("");
    setRecursiveDeleteID("");
    setShowCatalogManagement(false);
  };

  return (
    <>
      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        {/* 1. 向量知识库区域 */}
        <View style={styles.protoSectionHeader}>
          <Text style={styles.protoSectionTitle}>向量知识库</Text>
          <Pressable
            disabled={pendingKnowledge}
            onPress={() => {
              setManagedCategoryID("");
              setRecursiveDeleteID("");
              setShowCatalogManagement(true);
            }}
            style={({ pressed }) =>
              buttonFeedback(
                [styles.protoNewKbBtn, pendingKnowledge && styles.disabledButton],
                pressed,
              )
            }
          >
            <Text style={styles.protoNewKbBtnText}>+ 新建知识库</Text>
          </Pressable>
        </View>

        {message ? <Text style={styles.statusMessage}>{message}</Text> : null}

        {/* 知识库卡片流 */}
        <View style={styles.kbList}>
          {sortedBases.length === 0 ? (
            <View style={styles.emptyCard}>
              <Text style={styles.emptyCardTitle}>暂无向量知识库</Text>
              <Text style={styles.emptyCardSubtitle}>
                点击右上角“+ 新建知识库”开始添加
              </Text>
            </View>
          ) : (
            sortedBases.map((kb) => {
              const isSelected = kb.id === selectedID;
              const docCount = isSelected ? documents.length : 0;
              const chunkCount = isSelected
                ? jobs.reduce((acc, job) => acc + (job.total_chunks || 0), 0)
                : 0;

              return (
                <Pressable
                  key={kb.id}
                  onPress={() => selectBase(kb.id)}
                  style={({ pressed }) =>
                    buttonFeedback(
                      [styles.kbCard, isSelected && styles.kbCardSelected],
                      pressed,
                    )
                  }
                >
                  <View style={styles.kbHeaderRow}>
                    <View style={styles.kbTitleGroup}>
                      <Text style={styles.kbIcon}>📑</Text>
                      <Text numberOfLines={1} style={styles.kbTitle}>
                        {kb.name}
                      </Text>
                    </View>
                    <View style={styles.connectedBadge}>
                      <Text style={styles.connectedText}>
                        {isSelected ? "已连接" : "就绪"}
                      </Text>
                    </View>
                  </View>

                  <Text numberOfLines={2} style={styles.kbDesc}>
                    {kb.description || "涵盖相关业务文档与向量检索切片数据"}
                  </Text>

                  <View style={styles.kbFooterRow}>
                    <Text style={styles.kbStatText}>
                      {docCount > 0 ? `${docCount} 篇文档` : "文档已同步"}
                    </Text>
                    <Text style={styles.kbStatText}>
                      {chunkCount > 0 ? `${chunkCount} 个向量分块` : "已连接"}
                    </Text>
                    <Pressable
                      hitSlop={8}
                      onPress={(e) => {
                        e.stopPropagation();
                        openBaseSettings(kb.id);
                      }}
                      style={styles.kbManageBtn}
                    >
                      <Text style={styles.kbManageBtnText}>⚙ 管理</Text>
                    </Pressable>
                  </View>
                </Pressable>
              );
            })
          )}
        </View>

        {/* 2. 已索引文档区域 */}
        <View style={styles.docPanel}>
          <View style={styles.docPanelHeader}>
            <Text style={styles.docPanelTitle}>已索引文档</Text>
            <Pressable
              disabled={pendingKnowledge}
              onPress={() => {
                if (selectedBase) {
                  onUploadDocument();
                } else if (sortedBases.length > 0) {
                  selectBase(sortedBases[0].id);
                  onUploadDocument();
                } else {
                  setShowCatalogManagement(true);
                }
              }}
              style={({ pressed }) =>
                buttonFeedback(
                  [styles.uploadDocBtn, pendingKnowledge && styles.disabledButton],
                  pressed,
                )
              }
            >
              <Text style={styles.uploadDocBtnText}>上传文档</Text>
            </Pressable>
          </View>

          {documents.length === 0 ? (
            <View style={styles.emptyDocBox}>
              <Text style={styles.emptyDocText}>
                {selectedBase
                  ? "当前知识库暂无文档，可点击右上角“上传文档”"
                  : "请选择或新建一个知识库以查看文档"}
              </Text>
            </View>
          ) : (
            documents.map((doc, idx) => {
              const isReady = doc.status === "ready";
              const isFailed = doc.status === "failed";
              const sizeLabel = doc.content_type || "通用文档";

              return (
                <View
                  key={doc.id}
                  style={[
                    styles.docItemRow,
                    idx === documents.length - 1 && styles.docItemRowLast,
                  ]}
                >
                  <View style={styles.docInfo}>
                    <Text numberOfLines={1} style={styles.docTitle}>
                      📄 {doc.file_name}
                    </Text>
                    <Text style={styles.docMeta}>
                      {sizeLabel}
                      {doc.updated_at ? ` · ${formatDate(doc.updated_at)}` : ""}
                    </Text>
                  </View>
                  <View style={styles.docActions}>
                    <View
                      style={[
                        styles.docStatusBadge,
                        isReady && styles.docStatusBadgeReady,
                        isFailed && styles.docStatusBadgeFailed,
                      ]}
                    >
                      <Text
                        style={[
                          styles.docStatusText,
                          isReady && styles.docStatusTextReady,
                          isFailed && styles.docStatusTextFailed,
                        ]}
                      >
                        {isReady ? "已解析" : isFailed ? "失败" : "解析中"}
                      </Text>
                    </View>
                    <Pressable
                      hitSlop={6}
                      onPress={() => {
                        if (selectedBase) onDeleteDocument(selectedBase.id, doc.id);
                      }}
                      style={styles.docDeleteBtn}
                    >
                      <Text style={styles.docDeleteBtnText}>✕</Text>
                    </Pressable>
                  </View>
                </View>
              );
            })
          )}
        </View>

      <View style={styles.sessionSection}>
        <Pressable
          onPress={() => setShowSessionRAG((current) => !current)}
          style={({ pressed }) => buttonFeedback(styles.sessionSummaryRow, pressed)}
        >
          <View style={styles.flex}>
            <Text style={styles.sectionTitle}>当前会话检索</Text>
            <Text style={styles.meta}>
              {modeLabel(settings.mode)} · Top K {settings.top_k}
            </Text>
          </View>
          {pendingSettings ? <ButtonContent loading text="保存中" /> : null}
          <Text style={styles.disclosure}>{showSessionRAG ? "收起" : "设置"}</Text>
        </Pressable>

        {showSessionRAG ? (
          <View style={styles.sessionSettingsBody}>
            {!canUpdateRAG ? <Text style={styles.errorText}>请先连接并加载会话</Text> : null}
            <View style={styles.modeRow}>
              {(["off", "manual", "auto", "always"] as const).map((mode) => (
                <Pressable
                  disabled={!canUpdateRAG || pendingSettings || settings.mode === mode}
                  key={mode}
                  onPress={() => updateRAG({ mode })}
                  style={({ pressed }) =>
                    buttonFeedback(
                      [
                        styles.modeButton,
                        settings.mode === mode && styles.modeButtonActive,
                        (!canUpdateRAG || pendingSettings) && styles.disabledButton,
                      ],
                      pressed,
                    )
                  }
                >
                  {pendingSettings && pendingRAGMode === mode ? (
                    <ActivityIndicator color="#12100e" size="small" />
                  ) : null}
                  <Text style={[styles.modeText, settings.mode === mode && styles.modeTextActive]}>
                    {modeLabel(mode)}
                  </Text>
                </Pressable>
              ))}
            </View>
            <View style={styles.rowBetween}>
              <Text style={styles.meta}>Top K</Text>
              <TextInput
                editable={canUpdateRAG && !pendingSettings}
                keyboardType="number-pad"
                onChangeText={setTopKDraft}
                onEndEditing={commitTopK}
                style={[styles.smallInput, (!canUpdateRAG || pendingSettings) && styles.disabledButton]}
                value={topKDraft}
              />
            </View>
            <Text style={styles.meta}>检索范围</Text>
            <View style={styles.chipWrap}>
              {categories.map((category) => (
                <Pressable
                  disabled={!canUpdateRAG || pendingSettings}
                  key={category.id}
                  onPress={() =>
                    updateRAG({ category_ids: toggleID(settings.category_ids, category.id) })
                  }
                  style={({ pressed }) =>
                    buttonFeedback(
                      [
                        styles.scopeChip,
                        settings.category_ids.includes(category.id) && styles.scopeChipActive,
                        (!canUpdateRAG || pendingSettings) && styles.disabledButton,
                      ],
                      pressed,
                    )
                  }
                >
                  <Text style={styles.scopeText}>
                    {settings.category_ids.includes(category.id) ? "✓ " : ""}
                    {categoryPathLabel(category, categories)}
                  </Text>
                </Pressable>
              ))}
              {sortedBases.map((base) => (
                <Pressable
                  disabled={!canUpdateRAG || pendingSettings}
                  key={base.id}
                  onPress={() =>
                    updateRAG({ knowledge_base_ids: toggleID(settings.knowledge_base_ids, base.id) })
                  }
                  style={({ pressed }) =>
                    buttonFeedback(
                      [
                        styles.scopeChip,
                        settings.knowledge_base_ids.includes(base.id) && styles.scopeChipActive,
                        (!canUpdateRAG || pendingSettings) && styles.disabledButton,
                      ],
                      pressed,
                    )
                  }
                >
                  <Text style={styles.scopeText}>
                    {settings.knowledge_base_ids.includes(base.id) ? "✓ " : ""}
                    {base.name}
                  </Text>
                </Pressable>
              ))}
            </View>
          </View>
        ) : null}
        </View>
      </ScrollView>

      <ResponsiveFormModal
        buttonFeedback={buttonFeedback}
        footer={(
          <Pressable onPress={closeCatalogManagement} style={({ pressed }) => buttonFeedback(styles.footerDoneButton, pressed)}>
            <Text style={styles.footerDoneText}>完成</Text>
          </Pressable>
        )}
        onClose={closeCatalogManagement}
        title={managedCategory ? "管理目录" : "新建目录或知识库"}
        visible={showCatalogManagement}
      >
        <CatalogManagement
          availableMoveParents={availableMoveParents}
          baseName={baseName}
          buttonFeedback={buttonFeedback}
          categories={categories}
          categoryMoveParentID={categoryMoveParentID}
          categoryName={categoryName}
          managedCategory={managedCategory}
          onCreateCategory={() => {
            if (onCreateCategory(categoryName, parentID)) setCategoryName("");
          }}
          onCreateKnowledgeBase={() => {
            if (onCreateKnowledgeBase(baseName, parentID, profileID)) setBaseName("");
          }}
          onDeleteCategory={onDeleteCategory}
          onMoveCategory={onMoveCategory}
          onSelectMoveParent={setCategoryMoveParentID}
          onSelectParent={setParentID}
          onSelectProfile={setProfileID}
          parentID={parentID}
          pending={pendingKnowledge}
          profileID={profileID}
          profiles={profiles}
          recursiveDeleteID={recursiveDeleteID}
          setBaseName={setBaseName}
          setCategoryName={setCategoryName}
          setRecursiveDeleteID={setRecursiveDeleteID}
        />
      </ResponsiveFormModal>

      {selectedBase ? (
        <ResponsiveFormModal
          buttonFeedback={buttonFeedback}
          footer={(
            <Pressable onPress={() => setDetailTab(null)} style={({ pressed }) => buttonFeedback(styles.footerDoneButton, pressed)}>
              <Text style={styles.footerDoneText}>完成</Text>
            </Pressable>
          )}
          onClose={() => setDetailTab(null)}
          title={selectedBase.name}
          visible={Boolean(detailTab)}
        >
          <Text style={styles.meta}>{selectedBase.description || "未填写描述"}</Text>
          <View style={styles.tabRow}>
            {(["search", "config"] as const).map((tab) => (
              <Pressable
                key={tab}
                onPress={() => setDetailTab(tab)}
                style={({ pressed }) =>
                  buttonFeedback([styles.tabButton, detailTab === tab && styles.tabButtonActive], pressed)
                }
              >
                <Text style={[styles.tabText, detailTab === tab && styles.tabTextActive]}>{tab === "search" ? "检索测试" : "知识库设置"}</Text>
              </Pressable>
            ))}
          </View>
          {detailTab === "search" ? (
            <SearchPanel
              buttonFeedback={buttonFeedback}
              onChangeQuery={setSemanticQuery}
              onSearch={() =>
                onSearch(semanticQuery, {
                  ...settings,
                  category_ids: [],
                  knowledge_base_ids: [selectedBase.id],
                })
              }
              pending={pendingKnowledge}
              query={semanticQuery}
              result={searchResult}
            />
          ) : null}
          {detailTab === "config" ? (
            <ConfigPanel
              base={selectedBase}
              buttonFeedback={buttonFeedback}
              categories={categories}
              onDelete={(knowledgeBaseID) => {
                const sent = onDeleteKnowledgeBase(knowledgeBaseID);
                if (sent) setDetailTab(null);
                return sent;
              }}
              onUpdate={onUpdateKnowledgeBase}
              pending={pendingKnowledge}
              profiles={profiles}
            />
          ) : null}
        </ResponsiveFormModal>
      ) : null}
    </>
  );
}

type ExplorerProps = {
  bases: KnowledgeBase[];
  buttonFeedback: ButtonFeedback;
  categories: KnowledgeCategory[];
  documents: KnowledgeDocument[];
  expanded: Record<string, boolean>;
  filterText: string;
  jobs: KnowledgeIndexingJob[];
  onDeleteDocument: (knowledgeBaseID: string, documentID: string) => void;
  onManageBase: (knowledgeBaseID: string) => void;
  onManageCategory: (category: KnowledgeCategory) => void;
  onRetryDocument: (knowledgeBaseID: string, jobID: string) => void;
  onSelectBase: (knowledgeBaseID: string) => void;
  onToggleCategory: (categoryID: string) => void;
  pending: boolean;
  selectedBaseID: string;
};

function KnowledgeExplorer({
  bases,
  buttonFeedback,
  categories,
  documents,
  expanded,
  filterText,
  jobs,
  onDeleteDocument,
  onManageBase,
  onManageCategory,
  onRetryDocument,
  onSelectBase,
  onToggleCategory,
  pending,
  selectedBaseID,
}: ExplorerProps) {
  const query = filterText.trim().toLocaleLowerCase();
  const roots = categories.filter((category) => !category.parent_id);

  const documentMatches = (document: KnowledgeDocument) =>
    !query || document.file_name.toLocaleLowerCase().includes(query);
  const baseMatches = (base: KnowledgeBase) =>
    !query ||
    base.name.toLocaleLowerCase().includes(query) ||
    (base.id === selectedBaseID && documents.some(documentMatches));

  const categoryMatches = (category: KnowledgeCategory): boolean => {
    if (!query || category.name.toLocaleLowerCase().includes(query)) return true;
    if (bases.some((base) => base.category_id === category.id && baseMatches(base))) return true;
    return categories.some(
      (candidate) => candidate.parent_id === category.id && categoryMatches(candidate),
    );
  };

  const renderDocuments = (base: KnowledgeBase, depth: number) => {
    if (base.id !== selectedBaseID) return null;
    const visibleDocuments = documents.filter(documentMatches);
    if (pending && documents.length === 0) {
      return (
        <View style={[styles.loadingRow, { paddingLeft: 48 + depth * 18 }]}>
          <ActivityIndicator color="#12100e" size="small" />
          <Text style={styles.meta}>加载文档中</Text>
        </View>
      );
    }
    if (visibleDocuments.length === 0) {
      return (
        <Text style={[styles.emptyInline, { paddingLeft: 48 + depth * 18 }]}>
          {query ? "没有匹配文档" : "暂无文档"}
        </Text>
      );
    }
    return visibleDocuments.map((document) => {
      const job = jobs.find((candidate) => candidate.document_id === document.id);
      const progress = job && job.total_chunks > 0
        ? Math.round((job.completed_chunks / job.total_chunks) * 100)
        : 0;
      return (
        <View key={document.id} style={[styles.documentRow, { paddingLeft: 48 + depth * 18 }]}>
          <View style={styles.documentMark} />
          <View style={styles.flex}>
            <Text numberOfLines={1} style={styles.documentName}>{document.file_name}</Text>
            <Text style={styles.documentMeta}>
              {documentStatusLabel(document.status)}
              {job ? ` · ${job.stage} ${progress}%` : ""}
              {document.updated_at ? ` · ${formatDate(document.updated_at)}` : ""}
            </Text>
            {document.failure_reason ? (
              <Text numberOfLines={2} style={styles.errorText}>{document.failure_reason}</Text>
            ) : null}
          </View>
          {document.status === "failed" && job ? (
            <Pressable
              disabled={pending}
              onPress={() => onRetryDocument(base.id, job.id)}
              style={({ pressed }) => buttonFeedback(styles.rowAction, pressed)}
            >
              <Text style={styles.rowActionText}>重试</Text>
            </Pressable>
          ) : null}
          <Pressable
            disabled={pending}
            onPress={() => onDeleteDocument(base.id, document.id)}
            style={({ pressed }) => buttonFeedback(styles.rowAction, pressed)}
          >
            <Text style={styles.deleteText}>删除</Text>
          </Pressable>
        </View>
      );
    });
  };

  const renderBase = (base: KnowledgeBase, depth: number) => {
    if (!baseMatches(base)) return null;
    const selected = base.id === selectedBaseID;
    return (
      <View key={base.id}>
        <View style={[styles.baseRow, selected && styles.baseRowActive, { paddingLeft: 26 + depth * 18 }]}>
          <Pressable
            onPress={() => onSelectBase(base.id)}
            style={({ pressed }) => buttonFeedback(styles.baseMain, pressed)}
          >
            <View style={styles.baseMark} />
            <Text numberOfLines={1} style={styles.baseName}>{base.name}</Text>
            <Text style={styles.rowMeta}>{selected ? documents.length : ""}</Text>
            <Text style={[styles.statusPill, base.rag_enabled ? styles.statusPillOn : styles.statusPillOff]}>
              {base.rag_enabled ? "已启用" : "关闭"}
            </Text>
          </Pressable>
          <Pressable
            onPress={() => onManageBase(base.id)}
            style={({ pressed }) => buttonFeedback(styles.rowAction, pressed)}
          >
            <Text style={styles.rowActionText}>设置</Text>
          </Pressable>
        </View>
        {selected ? renderDocuments(base, depth) : null}
      </View>
    );
  };

  const renderCategory = (category: KnowledgeCategory, depth: number): React.ReactNode => {
    if (!categoryMatches(category)) return null;
    const children = categories.filter((candidate) => candidate.parent_id === category.id);
    const categoryBases = bases.filter((base) => base.category_id === category.id);
    const open = query ? true : expanded[category.id] !== false;
    return (
      <View key={category.id}>
        <View style={[styles.categoryRow, { paddingLeft: 8 + depth * 18 }]}>
          <Pressable
            onPress={() => onToggleCategory(category.id)}
            style={({ pressed }) => buttonFeedback(styles.categoryMain, pressed)}
          >
            <Text style={styles.arrow}>{open ? "▾" : "›"}</Text>
            <View style={styles.folderMark} />
            <Text numberOfLines={1} style={styles.categoryName}>{category.name}</Text>
            <Text style={styles.rowMeta}>{categoryBases.length}</Text>
          </Pressable>
          <Pressable
            onPress={() => onManageCategory(category)}
            style={({ pressed }) => buttonFeedback(styles.rowAction, pressed)}
          >
            <Text style={styles.rowActionText}>管理</Text>
          </Pressable>
        </View>
        {open ? (
          <>
            {categoryBases.map((base) => renderBase(base, depth + 1))}
            {children.map((child) => renderCategory(child, depth + 1))}
          </>
        ) : null}
      </View>
    );
  };

  const rootBases = bases.filter((base) => !base.category_id);
  return (
    <View style={styles.tree}>
      {roots.map((category) => renderCategory(category, 0))}
      {rootBases.map((base) => renderBase(base, 0))}
    </View>
  );
}

type CatalogManagementProps = {
  availableMoveParents: KnowledgeCategory[];
  baseName: string;
  buttonFeedback: ButtonFeedback;
  categories: KnowledgeCategory[];
  categoryMoveParentID: string;
  categoryName: string;
  managedCategory?: KnowledgeCategory;
  onCreateCategory: () => void;
  onCreateKnowledgeBase: () => void;
  onDeleteCategory: (categoryID: string, recursive?: boolean) => boolean;
  onMoveCategory: (categoryID: string, parentID?: string) => boolean;
  onSelectMoveParent: (parentID: string) => void;
  onSelectParent: (parentID: string) => void;
  onSelectProfile: (profileID: string) => void;
  parentID: string;
  pending: boolean;
  profileID: string;
  profiles: KnowledgeIndexProfile[];
  recursiveDeleteID: string;
  setBaseName: (value: string) => void;
  setCategoryName: (value: string) => void;
  setRecursiveDeleteID: (value: string) => void;
};

function CatalogManagement({
  availableMoveParents,
  baseName,
  buttonFeedback,
  categories,
  categoryMoveParentID,
  categoryName,
  managedCategory,
  onCreateCategory,
  onCreateKnowledgeBase,
  onDeleteCategory,
  onMoveCategory,
  onSelectMoveParent,
  onSelectParent,
  onSelectProfile,
  parentID,
  pending,
  profileID,
  profiles,
  recursiveDeleteID,
  setBaseName,
  setCategoryName,
  setRecursiveDeleteID,
}: CatalogManagementProps) {
  return (
    <View style={styles.managementPanel}>
      {managedCategory ? (
        <View style={styles.managementBlock}>
          <Text style={styles.meta}>{categoryPathLabel(managedCategory, categories)}</Text>
          <Text style={styles.meta}>移动到</Text>
          <View style={styles.chipWrap}>
            <ScopeChip active={!categoryMoveParentID} label="根目录" onPress={() => onSelectMoveParent("")} />
            {availableMoveParents.map((category) => (
              <ScopeChip
                active={categoryMoveParentID === category.id}
                key={category.id}
                label={categoryPathLabel(category, categories)}
                onPress={() => onSelectMoveParent(category.id)}
              />
            ))}
          </View>
          <View style={styles.inlineActions}>
            <Pressable
              disabled={pending || categoryMoveParentID === (managedCategory.parent_id || "")}
              onPress={() => onMoveCategory(managedCategory.id, categoryMoveParentID)}
              style={({ pressed }) =>
                buttonFeedback(
                  [
                    styles.secondaryButton,
                    (pending || categoryMoveParentID === (managedCategory.parent_id || "")) && styles.disabledButton,
                  ],
                  pressed,
                )
              }
            >
              <Text style={styles.secondaryText}>移动目录</Text>
            </Pressable>
            <Pressable
              disabled={pending}
              onPress={() => onDeleteCategory(managedCategory.id, false)}
              style={({ pressed }) => buttonFeedback(styles.deleteOutlineButton, pressed)}
            >
              <Text style={styles.deleteText}>删除空目录</Text>
            </Pressable>
          </View>
          <Pressable
            disabled={pending}
            onPress={() => {
              if (recursiveDeleteID !== managedCategory.id) {
                setRecursiveDeleteID(managedCategory.id);
                return;
              }
              if (onDeleteCategory(managedCategory.id, true)) setRecursiveDeleteID("");
            }}
            style={({ pressed }) => buttonFeedback(styles.dangerButton, pressed)}
          >
            <Text style={styles.dangerText}>
              {recursiveDeleteID === managedCategory.id ? "再次确认递归删除" : "递归删除目录及知识库"}
            </Text>
          </Pressable>
        </View>
      ) : null}

      <View style={styles.managementBlock}>
        <Text style={styles.sectionTitle}>创建内容</Text>
        <Text style={styles.meta}>创建位置</Text>
        <View style={styles.chipWrap}>
          <ScopeChip active={!parentID} label="根目录" onPress={() => onSelectParent("")} />
          {categories.map((category) => (
            <ScopeChip
              active={parentID === category.id}
              key={category.id}
              label={categoryPathLabel(category, categories)}
              onPress={() => onSelectParent(category.id)}
            />
          ))}
        </View>
        <View style={styles.createRow}>
          <TextInput
            onChangeText={setCategoryName}
            placeholder="新目录"
            placeholderTextColor="#776f66"
            style={[styles.input, styles.flex]}
            value={categoryName}
          />
          <Pressable
            disabled={pending || !categoryName.trim()}
            onPress={onCreateCategory}
            style={({ pressed }) =>
              buttonFeedback(
                [styles.primaryButton, (pending || !categoryName.trim()) && styles.disabledButton],
                pressed,
              )
            }
          >
            <ButtonContent loading={pending} text="新建目录" />
          </Pressable>
        </View>
        {profiles.length > 0 ? (
          <>
            <Text style={styles.meta}>索引配置</Text>
            <View style={styles.chipWrap}>
              {profiles.filter((profile) => profile.status === "active").map((profile) => (
                <ScopeChip
                  active={profileID === profile.id}
                  key={profile.id}
                  label={profile.name}
                  onPress={() => onSelectProfile(profile.id)}
                />
              ))}
            </View>
          </>
        ) : null}
        <View style={styles.createRow}>
          <TextInput
            onChangeText={setBaseName}
            placeholder="新知识库"
            placeholderTextColor="#776f66"
            style={[styles.input, styles.flex]}
            value={baseName}
          />
          <Pressable
            disabled={pending || !baseName.trim()}
            onPress={onCreateKnowledgeBase}
            style={({ pressed }) =>
              buttonFeedback(
                [styles.primaryButton, (pending || !baseName.trim()) && styles.disabledButton],
                pressed,
              )
            }
          >
            <ButtonContent loading={pending} text="新建知识库" />
          </Pressable>
        </View>
      </View>
    </View>
  );
}

function ScopeChip({ active, label, onPress }: { active: boolean; label: string; onPress: () => void }) {
  return (
    <Pressable onPress={onPress} style={[styles.scopeChip, active && styles.scopeChipActive]}>
      <Text style={styles.scopeText}>{label}</Text>
    </Pressable>
  );
}

function SearchPanel({
  buttonFeedback,
  onChangeQuery,
  onSearch,
  pending,
  query,
  result,
}: {
  buttonFeedback: ButtonFeedback;
  onChangeQuery: (value: string) => void;
  onSearch: () => void;
  pending: boolean;
  query: string;
  result: KnowledgeSearchPreviewResultPayload | null;
}) {
  return (
    <View style={styles.detailBody}>
      <View style={styles.toolbarRow}>
        <TextInput
          onChangeText={onChangeQuery}
          onSubmitEditing={onSearch}
          placeholder="输入检索问题"
          placeholderTextColor="#776f66"
          style={[styles.input, styles.flex]}
          value={query}
        />
        <Pressable
          disabled={pending || !query.trim()}
          onPress={onSearch}
          style={({ pressed }) =>
            buttonFeedback(
              [styles.primaryButton, (pending || !query.trim()) && styles.disabledButton],
              pressed,
            )
          }
        >
          <ButtonContent loading={pending} text="检索" />
        </Pressable>
      </View>
      {result ? (
        <View style={styles.searchResults}>
          <Text style={styles.meta}>
            {result.hits.length} 个片段 · {result.profiles.length} 个索引配置
          </Text>
          {result.error ? <Text style={styles.errorText}>{result.error}</Text> : null}
          {result.hits.map((hit) => (
            <View key={hit.chunk_id} style={styles.hitRow}>
              <Text style={styles.hitTitle}>[{hit.rank}] {hit.source_name || hit.document_id}</Text>
              <Text selectable style={styles.hitText}>{hit.text}</Text>
            </View>
          ))}
        </View>
      ) : null}
    </View>
  );
}

function ConfigPanel({
  base,
  buttonFeedback,
  categories,
  onDelete,
  onUpdate,
  pending,
  profiles,
}: {
  base: KnowledgeBase;
  buttonFeedback: ButtonFeedback;
  categories: KnowledgeCategory[];
  onDelete: (knowledgeBaseID: string) => boolean;
  onUpdate: (base: KnowledgeBase, changes: KnowledgeBaseChanges) => boolean;
  pending: boolean;
  profiles: KnowledgeIndexProfile[];
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const activeProfiles = profiles.filter((profile) => profile.status === "active" && !profile.deleted);
  return (
    <View style={styles.detailBody}>
      <Text style={styles.meta}>所属目录</Text>
      <View style={styles.chipWrap}>
        <ScopeChip active={!base.category_id} label="根目录" onPress={() => onUpdate(base, { categoryID: "" })} />
        {categories.map((category) => (
          <ScopeChip
            active={base.category_id === category.id}
            key={category.id}
            label={categoryPathLabel(category, categories)}
            onPress={() => onUpdate(base, { categoryID: category.id })}
          />
        ))}
      </View>
      <View style={styles.rowBetween}>
        <View>
          <Text style={styles.meta}>RAG 状态</Text>
          <Text style={styles.documentName}>{base.rag_enabled ? "已启用" : "未启用"}</Text>
        </View>
        <Pressable
          disabled={pending || (!base.rag_enabled && !base.active_index_profile_id)}
          onPress={() => onUpdate(base, { ragEnabled: !base.rag_enabled })}
          style={({ pressed }) =>
            buttonFeedback(
              [
                styles.secondaryButton,
                (pending || (!base.rag_enabled && !base.active_index_profile_id)) && styles.disabledButton,
              ],
              pressed,
            )
          }
        >
          <Text style={styles.secondaryText}>{base.rag_enabled ? "关闭 RAG" : "启用 RAG"}</Text>
        </Pressable>
      </View>
      <Text style={styles.meta}>索引配置</Text>
      <View style={styles.chipWrap}>
        {activeProfiles.map((profile) => (
          <ScopeChip
            active={base.active_index_profile_id === profile.id}
            key={profile.id}
            label={profile.name}
            onPress={() => onUpdate(base, { activeIndexProfileID: profile.id, ragEnabled: true })}
          />
        ))}
      </View>
      <Pressable
        disabled={pending}
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

function normalizeRAG(value?: RAGSettings): RAGSettings {
  return {
    mode: value?.mode || "auto",
    knowledge_base_ids: value?.knowledge_base_ids || [],
    category_ids: value?.category_ids || [],
    top_k: value?.top_k || 8,
  };
}

function modeLabel(mode: string) {
  return ({ off: "关闭", manual: "手动", auto: "自动", always: "每轮" } as Record<string, string>)[mode] || mode;
}

function categoryPathLabel(category: KnowledgeCategory, categories: KnowledgeCategory[]) {
  const names = (category.ancestor_ids || [])
    .map((id) => categories.find((candidate) => candidate.id === id)?.name)
    .filter(Boolean);
  return [...names, category.name].join(" / ");
}

function documentStatusLabel(status: string) {
  return ({
    uploaded: "已上传",
    parsing: "解析中",
    chunking: "分块中",
    embedding: "向量化中",
    indexing: "索引中",
    ready: "已就绪",
    failed: "失败",
    deleted: "已删除",
  } as Record<string, string>)[status] || status;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("zh-CN", { month: "numeric", day: "numeric" });
}

const styles = StyleSheet.create({
  content: { gap: 12, padding: 14, paddingBottom: 28 },
  headerRow: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 10 },
  toolbarRow: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 8 },
  sectionHeader: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 10, justifyContent: "space-between" },
  explorerSection: {
    backgroundColor: "#fffdf7",
    borderColor: "#25231f",
    borderRadius: 16,
    borderWidth: 2,
    elevation: 3,
    overflow: "hidden",
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 3 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  sessionSection: {
    backgroundColor: "#fffdf7",
    borderColor: "#25231f",
    borderRadius: 16,
    borderWidth: 2,
    elevation: 3,
    overflow: "hidden",
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 3 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  sessionSummaryRow: { alignItems: "center", flexDirection: "row", gap: 10, minHeight: 58, paddingHorizontal: 12, paddingVertical: 10 },
  sessionSettingsBody: { borderTopColor: "#ded2c3", borderTopWidth: 1, gap: 10, padding: 12 },
  detailBody: { gap: 10 },
  tree: { borderTopColor: "#ded2c3", borderTopWidth: 1, marginTop: 10 },
  flex: { flex: 1, minWidth: 0 },
  eyebrow: { color: "#6c665f", fontSize: 11, fontWeight: "900" },
  title: { color: "#12100e", fontSize: 24, fontWeight: "900" },
  sectionTitle: { color: "#12100e", fontSize: 16, fontWeight: "900" },
  meta: { color: "#6c665f", fontSize: 12, fontWeight: "700" },
  input: { backgroundColor: "#fffdf7", borderColor: "#25231f", borderRadius: 8, borderWidth: 1.5, color: "#12100e", minHeight: 42, minWidth: 0, paddingHorizontal: 10, paddingVertical: 8 },
  smallInput: { backgroundColor: "#fffdf7", borderColor: "#25231f", borderRadius: 6, borderWidth: 1.5, color: "#12100e", maxWidth: "55%", minWidth: 72, paddingHorizontal: 8, paddingVertical: 5, textAlign: "right", width: 120 },
  primaryButton: { alignItems: "center", backgroundColor: "#ffd84f", borderColor: "#12100e", borderRadius: 8, borderWidth: 2, justifyContent: "center", minHeight: 42, paddingHorizontal: 11, paddingVertical: 8 },
  outlineButton: { alignItems: "center", backgroundColor: "#ffd84f", borderColor: "#12100e", borderRadius: 8, borderWidth: 2, justifyContent: "center", minHeight: 40, paddingHorizontal: 10, paddingVertical: 7 },
  secondaryButton: { alignItems: "center", backgroundColor: "#4fd7ee", borderColor: "#12100e", borderRadius: 8, borderWidth: 2, paddingHorizontal: 9, paddingVertical: 7 },
  secondaryText: { color: "#12100e", fontSize: 11, fontWeight: "900" },
  iconTextButton: { paddingHorizontal: 6, paddingVertical: 6 },
  iconTextButtonLabel: { color: "#6c665f", fontSize: 12, fontWeight: "900" },
  newButton: { alignItems: "center", backgroundColor: "#ffd84f", borderColor: "#12100e", borderRadius: 8, borderWidth: 2, justifyContent: "center", minHeight: 34, paddingHorizontal: 10, paddingVertical: 6 },
  newButtonText: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  disabledButton: { opacity: 0.42 },
  statusMessage: { backgroundColor: "#fffdf7", borderColor: "#25231f", borderRadius: 8, borderWidth: 1.5, color: "#3d3833", fontSize: 12, fontWeight: "800", paddingHorizontal: 10, paddingVertical: 8 },
  emptyText: { color: "#6c665f", paddingHorizontal: 12, paddingVertical: 18, textAlign: "center" },
  emptyInline: { color: "#8a8177", fontSize: 12, paddingBottom: 10, paddingTop: 8 },
  loadingRow: { alignItems: "center", flexDirection: "row", gap: 8, paddingBottom: 10, paddingTop: 8 },
  categoryRow: { alignItems: "center", borderBottomColor: "#ebe2d6", borderBottomWidth: 1, flexDirection: "row", minHeight: 46, paddingRight: 8 },
  categoryMain: { alignItems: "center", flex: 1, flexDirection: "row", gap: 8, minHeight: 46, minWidth: 0 },
  arrow: { color: "#776f66", fontSize: 16, textAlign: "center", width: 14 },
  folderMark: { backgroundColor: "#ffd15c", borderBottomLeftRadius: 2, borderBottomRightRadius: 2, borderTopRightRadius: 2, height: 12, width: 17 },
  categoryName: { color: "#12100e", flex: 1, fontSize: 14, fontWeight: "900" },
  baseRow: { alignItems: "center", borderBottomColor: "#ebe2d6", borderBottomWidth: 1, flexDirection: "row", minHeight: 44, paddingRight: 8 },
  baseRowActive: { backgroundColor: "#f5eedf" },
  baseMain: { alignItems: "center", flex: 1, flexDirection: "row", gap: 8, minHeight: 44, minWidth: 0 },
  baseMark: { backgroundColor: "#4da8ed", borderRadius: 2, height: 16, width: 13 },
  baseName: { color: "#24211f", flex: 1, fontSize: 13, fontWeight: "800" },
  rowMeta: { color: "#8a8177", fontSize: 11, minWidth: 18, textAlign: "right" },
  rowAction: { paddingHorizontal: 7, paddingVertical: 7 },
  rowActionText: { color: "#6c665f", fontSize: 11, fontWeight: "900" },
  statusPill: { borderRadius: 4, fontSize: 9, fontWeight: "900", overflow: "hidden", paddingHorizontal: 5, paddingVertical: 2 },
  statusPillOn: { backgroundColor: "#b9e9b0", color: "#245b2e" },
  statusPillOff: { backgroundColor: "#e5ddd2", color: "#6c665f" },
  documentRow: { alignItems: "center", borderBottomColor: "#ebe2d6", borderBottomWidth: 1, flexDirection: "row", gap: 8, minHeight: 54, paddingRight: 8, paddingVertical: 7 },
  documentMark: { backgroundColor: "#e9f4ff", borderColor: "#2387d8", borderRadius: 2, borderWidth: 1, height: 18, width: 14 },
  documentName: { color: "#12100e", fontSize: 13, fontWeight: "900" },
  documentMeta: { color: "#8a8177", fontSize: 10, fontWeight: "700", marginTop: 2 },
  managementPanel: { gap: 18 },
  managementBlock: { gap: 10 },
  chipWrap: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
  scopeChip: { borderColor: "#b8aea1", borderRadius: 6, borderWidth: 1, paddingHorizontal: 9, paddingVertical: 7 },
  scopeChipActive: { backgroundColor: "#b9e9b0", borderColor: "#12100e" },
  scopeText: { color: "#12100e", fontSize: 12, fontWeight: "800" },
  createRow: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 8 },
  inlineActions: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 8 },
  rowBetween: { alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 8, justifyContent: "space-between" },
  deleteOutlineButton: { alignItems: "center", borderColor: "#b5412b", borderRadius: 6, borderWidth: 1, paddingHorizontal: 9, paddingVertical: 7 },
  deleteText: { color: "#b5412b", fontSize: 11, fontWeight: "900" },
  dangerButton: { alignItems: "center", backgroundColor: "#ef7868", borderColor: "#12100e", borderRadius: 6, borderWidth: 2, paddingHorizontal: 10, paddingVertical: 9 },
  dangerText: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  errorText: { color: "#b5412b", fontSize: 11, fontWeight: "700" },
  tabRow: { backgroundColor: "#e8e5df", borderRadius: 7, flexDirection: "row", gap: 3, padding: 3 },
  tabButton: { alignItems: "center", borderRadius: 5, flex: 1, justifyContent: "center", minHeight: 38, paddingHorizontal: 8, paddingVertical: 7 },
  tabButtonActive: { backgroundColor: "#ffffff", borderColor: "#b8aea1", borderWidth: 1 },
  tabText: { color: "#6c665f", fontSize: 12, fontWeight: "900" },
  tabTextActive: { color: "#12100e" },
  footerDoneButton: { alignItems: "center", backgroundColor: "#ffd84f", borderColor: "#12100e", borderRadius: 6, borderWidth: 2, justifyContent: "center", minHeight: 42, minWidth: 104, paddingHorizontal: 16 },
  footerDoneText: { color: "#12100e", fontSize: 13, fontWeight: "900" },
  searchResults: { gap: 8 },
  hitRow: { borderTopColor: "#ded2c3", borderTopWidth: 1, gap: 4, paddingTop: 10 },
  hitTitle: { color: "#12100e", fontSize: 12, fontWeight: "900" },
  hitText: { color: "#3d3833", fontSize: 12, lineHeight: 18 },
  disclosure: { color: "#6c665f", fontSize: 12, fontWeight: "900" },
  modeRow: { flexDirection: "row", gap: 6, width: "100%" },
  modeButton: { alignItems: "center", borderColor: "#b8aea1", borderRadius: 6, borderWidth: 2, flexBasis: 0, flexDirection: "row", flexGrow: 1, flexShrink: 1, gap: 4, justifyContent: "center", minHeight: 40, minWidth: 0, paddingHorizontal: 4, paddingVertical: 8 },
  modeButtonActive: { backgroundColor: "#ffd84f", borderColor: "#12100e" },
  modeText: { color: "#6c665f", fontSize: 12, fontWeight: "900" },
  modeTextActive: { color: "#12100e" },
  protoSectionHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    marginBottom: 4,
    paddingHorizontal: 2,
  },
  protoSectionTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  protoNewKbBtn: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    elevation: 2,
    justifyContent: "center",
    paddingHorizontal: 12,
    paddingVertical: 6,
    shadowColor: "#12100e",
    shadowOffset: { height: 1.5, width: 0 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  protoNewKbBtnText: {
    color: "#12100e",
    fontSize: 11.5,
    fontWeight: "900",
  },
  kbList: {
    gap: 10,
  },
  kbCard: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 14,
    borderWidth: 2,
    elevation: 2,
    gap: 6,
    padding: 13,
    shadowColor: "#12100e",
    shadowOffset: { height: 2, width: 0 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  kbCardSelected: {
    borderColor: "#12100e",
    borderWidth: 2.5,
  },
  kbHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  kbTitleGroup: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 6,
    marginRight: 8,
  },
  kbIcon: {
    fontSize: 14,
  },
  kbTitle: {
    color: "#12100e",
    flex: 1,
    fontSize: 14,
    fontWeight: "900",
  },
  connectedBadge: {
    alignItems: "center",
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 1.5,
    justifyContent: "center",
    paddingHorizontal: 8,
    paddingVertical: 2,
  },
  connectedText: {
    color: "#0e4e16",
    fontSize: 10,
    fontWeight: "800",
  },
  kbDesc: {
    color: "#6c665f",
    fontSize: 11.5,
    lineHeight: 16,
  },
  kbFooterRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 12,
    marginTop: 2,
  },
  kbStatText: {
    color: "#8c857b",
    fontSize: 11,
    fontWeight: "700",
  },
  kbManageBtn: {
    marginLeft: "auto",
    paddingHorizontal: 4,
    paddingVertical: 2,
  },
  kbManageBtnText: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
  },
  emptyCard: {
    alignItems: "center",
    backgroundColor: "#fffdf7",
    borderColor: "#ded7cc",
    borderRadius: 14,
    borderStyle: "dashed",
    borderWidth: 2,
    padding: 24,
  },
  emptyCardTitle: {
    color: "#12100e",
    fontSize: 13.5,
    fontWeight: "900",
  },
  emptyCardSubtitle: {
    color: "#8c857b",
    fontSize: 11,
    fontWeight: "700",
    marginTop: 4,
  },
  docPanel: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 14,
    borderWidth: 2,
    elevation: 2,
    gap: 8,
    padding: 13,
    shadowColor: "#12100e",
    shadowOffset: { height: 2, width: 0 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  docPanelHeader: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingBottom: 4,
  },
  docPanelTitle: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
  },
  uploadDocBtn: {
    alignItems: "center",
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    justifyContent: "center",
    paddingHorizontal: 10,
    paddingVertical: 4,
  },
  uploadDocBtnText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  emptyDocBox: {
    paddingVertical: 14,
  },
  emptyDocText: {
    color: "#8c857b",
    fontSize: 11.5,
    fontWeight: "700",
    textAlign: "center",
  },
  docItemRow: {
    alignItems: "center",
    borderBottomColor: "#ede7da",
    borderBottomWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: 8,
  },
  docItemRowLast: {
    borderBottomWidth: 0,
  },
  docInfo: {
    flex: 1,
    gap: 3,
    marginRight: 8,
  },
  docTitle: {
    color: "#12100e",
    fontSize: 12.5,
    fontWeight: "800",
  },
  docMeta: {
    color: "#8c857b",
    fontSize: 10,
    fontWeight: "700",
  },
  docActions: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  docStatusBadge: {
    alignItems: "center",
    backgroundColor: "#f0eae1",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 1.5,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  docStatusBadgeReady: {
    backgroundColor: "#b9e9b0",
  },
  docStatusBadgeFailed: {
    backgroundColor: "#ff7f68",
  },
  docStatusText: {
    color: "#6c665f",
    fontSize: 9.5,
    fontWeight: "800",
  },
  docStatusTextReady: {
    color: "#0e4e16",
  },
  docStatusTextFailed: {
    color: "#7a1f1a",
  },
  docDeleteBtn: {
    padding: 4,
  },
  docDeleteBtnText: {
    color: "#8c857b",
    fontSize: 12,
    fontWeight: "900",
  },
});
