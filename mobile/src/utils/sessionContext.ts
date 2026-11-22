import type { ContextInfo, RequestContext, SessionSummary } from "../protocol";
import { formatTokenCount } from "./tokenUsage";

const inputCategories = [
  ["message_tokens", "消息"],
  ["system_tokens", "系统提示词"],
  ["skill_tokens", "技能"],
  ["mcp_tool_tokens", "MCP 工具"],
  ["other_tool_tokens", "其他工具"],
  ["other_tokens", "其他"],
] as const satisfies ReadonlyArray<readonly [keyof RequestContext, string]>;

export function describeSessionContext(context?: ContextInfo, sessionWindowK?: SessionSummary["context_window_k"]) {
  const snapshotUsed = context?.selected_tokens;
  const windowK = context?.window_k || sessionWindowK;
  const snapshotWindow = windowK ? windowK * 1000 : undefined;
  const sample = context?.request_context;
  const categories = sample ? inputCategories.map(([key, label]) => ({ label, tokens: sample[key] })) : [];
  const used = sample ? categories.reduce((total, item) => total + item.tokens, 0) : snapshotUsed;
  const window = sample?.window_tokens ?? snapshotWindow;

  return { sample, categories, used, window, snapshotUsed, snapshotWindow };
}

export function formatContextUsage(used?: number, window?: number, exact = false): string {
  const format = (tokens?: number) => tokens === undefined ? "—" : exact ? tokens.toLocaleString() : formatTokenCount(tokens);
  return `${format(used)} / ${format(window)}`;
}

export type ContextUpdateSource = "snapshot" | "live";

export function mergeSessionContext(
  previous: ContextInfo | undefined,
  incoming: ContextInfo,
  source: ContextUpdateSource,
  keepLiveSample = false,
): ContextInfo {
  const merged = {
    ...(source === "live" ? previous : undefined),
    ...incoming,
    request_context: keepLiveSample ? previous?.request_context : incoming.request_context ?? previous?.request_context,
  };
  if (source === "live" && incoming.summary === undefined) {
    // Completion events can omit display text while still reporting summary metadata.
    const checkpoint = incoming.checkpoint;
    if (!checkpoint && (incoming.has_summary === false || (incoming.summary_version === 0 && !incoming.has_summary))) {
      delete merged.summary;
      delete merged.checkpoint;
      merged.has_summary = false;
      merged.summary_version = incoming.summary_version ?? 0;
      merged.summary_hash = incoming.summary_hash;
    } else if (checkpoint && (checkpoint.version !== previous?.checkpoint?.version || checkpoint.summary !== previous.checkpoint.summary)) {
      merged.summary = checkpoint.summary;
    }
  }
  return merged;
}
