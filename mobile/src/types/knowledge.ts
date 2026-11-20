export type KnowledgeBaseChanges = {
  activeIndexProfileID?: string;
  categoryID?: string;
  ragEnabled?: boolean;
};

export type DocumentImportStatus = {
  phase: "importing" | "unknown" | "checking" | "review" | "complete";
  knowledgeBaseID: string;
  knowledgeBaseName: string;
  fileName: string;
  message: string;
};
