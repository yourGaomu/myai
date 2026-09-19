import type { TokenUsage } from "../protocol";

export function tokenCount(value?: number) {
  return typeof value === "number" ? `${value}` : "n/a";
}

export function usageHasValues(usage: TokenUsage) {
  return [
    usage.prompt_tokens,
    usage.completion_tokens,
    usage.total_tokens,
    usage.reasoning_tokens,
    usage.prompt_cached_tokens,
  ].some((value) => typeof value === "number");
}

export function formatTokenCount(num?: number): string {
  if (typeof num !== "number") return "n/a";
  if (num >= 1000) {
    return (num / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  }
  return `${num}`;
}

export function usageSummary(usage: TokenUsage) {
  const total = usage.total_tokens ?? ((usage.prompt_tokens || 0) + (usage.completion_tokens || 0));
  if (usage.prompt_tokens !== undefined && usage.completion_tokens !== undefined) {
    return `${formatTokenCount(total)} tokens (入 ${formatTokenCount(usage.prompt_tokens)} · 出 ${formatTokenCount(usage.completion_tokens)})`;
  }
  if (total !== undefined) {
    return `${formatTokenCount(total)} tokens`;
  }
  return "Usage unavailable";
}

export function usageBreakdown(usage?: TokenUsage) {
  if (!usage || !usageHasValues(usage)) return null;
  const total = usage.total_tokens ?? ((usage.prompt_tokens || 0) + (usage.completion_tokens || 0));
  const hasBreakdown = usage.prompt_tokens !== undefined && usage.completion_tokens !== undefined;
  return {
    total: formatTokenCount(total),
    rawTotal: total,
    input: formatTokenCount(usage.prompt_tokens),
    output: formatTokenCount(usage.completion_tokens),
    hasBreakdown,
  };
}
