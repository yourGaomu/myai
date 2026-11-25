import type {
  UserQuestionItemPayload,
  UserQuestionStructuredAnswerPayload,
} from "../protocol";
import type {
  UserQuestionAnswerSubmission,
  UserQuestionDraftAnswer,
} from "../types/app";

export function validateQuestionChoices(item: UserQuestionItemPayload): void {
  if (!item || typeof item.id !== "string" || !item.id.trim() || typeof item.prompt !== "string" || !item.prompt.trim()) {
    throw new Error("Invalid user question: id and prompt are required.");
  }
  if (Object.keys(item).some((key) => !["id", "prompt", "choices", "multi_select", "allow_other"].includes(key)) ||
      typeof item.multi_select !== "boolean" || typeof item.allow_other !== "boolean" || !Array.isArray(item.choices)) {
    throw new Error("Unsupported user-question schema. Update the app and server together.");
  }
  if (item.choices.length > 8 || (!item.choices.length && !item.allow_other)) {
    throw new Error(`Question ${item.id} requires valid answer options.`);
  }
  const labels = new Set<string>();
  let recommended = 0;
  for (const choice of item.choices) {
    if (!choice || Object.keys(choice).some((key) => !["label", "description", "recommended"].includes(key)) || typeof choice.label !== "string" || !choice.label.trim()) throw new Error(`Question ${item.id} contains an invalid choice.`);
    const label = choice.label.trim();
    if (labels.has(label)) throw new Error(`Question ${item.id} contains duplicate choices.`);
    labels.add(label);
    if (choice.description !== undefined && typeof choice.description !== "string") throw new Error(`Question ${item.id} contains an invalid description.`);
    if (choice.recommended !== undefined && typeof choice.recommended !== "boolean") throw new Error(`Question ${item.id} contains an invalid recommendation.`);
    if (choice.recommended) recommended += 1;
  }
  if (!item.multi_select && recommended > 1) throw new Error(`Question ${item.id} has multiple recommended choices.`);
}

export function emptyQuestionDraft(): UserQuestionDraftAnswer {
  return { selected: [], other: "" };
}

export function applySingleSelectOption(label: string): UserQuestionDraftAnswer {
  return { selected: [label], other: "" };
}

export function applySingleSelectOther(text: string): UserQuestionDraftAnswer {
  return { selected: [], other: text };
}

export function toggleMultiSelectOption(draft: UserQuestionDraftAnswer | undefined, label: string): UserQuestionDraftAnswer {
  const current = draft || emptyQuestionDraft();
  const selected = current.selected.includes(label)
    ? current.selected.filter((item) => item !== label)
    : [...current.selected, label];
  return { selected, other: current.other };
}

export function updateMultiSelectOther(draft: UserQuestionDraftAnswer | undefined, text: string): UserQuestionDraftAnswer {
  return { ...(draft || emptyQuestionDraft()), other: text };
}

export function isQuestionDraftComplete(draft: UserQuestionDraftAnswer | undefined): boolean {
  return Boolean(draft?.selected.length || draft?.other.trim());
}

export function getRecommendedLabels(item: UserQuestionItemPayload): string[] {
  return item.choices.filter((choice) => choice.recommended).map((choice) => choice.label);
}

export function getQuestionDraft(drafts: Record<string, UserQuestionDraftAnswer>, id: string): UserQuestionDraftAnswer | undefined {
  return Object.prototype.hasOwnProperty.call(drafts, id) ? drafts[id] : undefined;
}

export function canAdoptRecommended(items: UserQuestionItemPayload[], drafts: Record<string, UserQuestionDraftAnswer>): boolean {
  const unanswered = items.filter((item) => !isQuestionDraftComplete(getQuestionDraft(drafts, item.id)));
  return unanswered.length > 0 && unanswered.every((item) => getRecommendedLabels(item).length > 0);
}

export function mergeWithRecommendedDefaults(
  items: UserQuestionItemPayload[],
  drafts: Record<string, UserQuestionDraftAnswer>,
): Record<string, UserQuestionDraftAnswer> {
  const defaults = items
    .filter((item) => !isQuestionDraftComplete(getQuestionDraft(drafts, item.id)))
    .map((item) => [item.id, { selected: getRecommendedLabels(item), other: "" }]);
  return { ...drafts, ...Object.fromEntries(defaults) };
}

export function formatQuestionAnswerSummary(item: UserQuestionItemPayload, draft: UserQuestionDraftAnswer | undefined): string {
  if (!draft) return "";
  const selected = item.choices.filter((choice) => draft.selected.includes(choice.label)).map((choice) => choice.label);
  const other = draft.other.trim();
  if (selected.length && other) return `${selected.join("、")}；补充：${other}`;
  return selected.join("、") || other;
}

export function buildUserQuestionSubmission(
  items: UserQuestionItemPayload[],
  drafts: Record<string, UserQuestionDraftAnswer>,
  adoptedDefaults = false,
): UserQuestionAnswerSubmission {
  const entries = items.map((item): [string, UserQuestionStructuredAnswerPayload] => {
    const draft = getQuestionDraft(drafts, item.id) || emptyQuestionDraft();
    const selected = item.choices.filter((choice) => draft.selected.includes(choice.label)).map((choice) => choice.label);
    const other = draft.other.trim();
    if (!selected.length && !other) throw new Error(`Question ${item.id} is unanswered.`);
    if (!item.multi_select && selected.length && other) throw new Error(`Question ${item.id} cannot combine a choice and custom text.`);
    if (!item.allow_other && other) throw new Error(`Question ${item.id} does not allow custom text.`);
    return [item.id, {
      ...(selected.length ? { selected } : {}),
      ...(other ? { other } : {}),
    }];
  });
  return { structuredAnswers: Object.fromEntries(entries), ...(adoptedDefaults ? { adoptedDefaults: true } : {}) };
}

export function formatSubmittedCardSummary(
  items: UserQuestionItemPayload[],
  submission: UserQuestionAnswerSubmission,
): string {
  const fullyAdopted = submission.adoptedDefaults && items.every((item) => {
    const answer = submission.structuredAnswers[item.id];
    const recommended = getRecommendedLabels(item);
    return Boolean(answer) && (answer.other || "").trim() === "" &&
      recommended.length === (answer.selected || []).length &&
      recommended.every((label) => (answer.selected || []).includes(label));
  });
  if (fullyAdopted) return "已按推荐方案提交";
  const summaries = items.map((item, index) => {
    const answer = submission.structuredAnswers[item.id];
    const draft = { selected: answer?.selected || [], other: answer?.other || "" };
    return `Q${index + 1}: ${formatQuestionAnswerSummary(item, draft)}`;
  });
  const summary = `${submission.adoptedDefaults ? "其余采用推荐 · " : ""}${summaries.join(" · ")}`;
  return summary.length > 180 ? `${summary.slice(0, 177)}…` : summary;
}
