import type { TokenUsage } from "../protocol";

export function tokenCount(value?: number) {
  return typeof value === "number" ? `${value}` : "n/a";
}

export function usageHasValues(usage: TokenUsage) {
  return Boolean(usage.prompt_cache_reported || usage.prompt_cache_incomplete) || [
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
  const breakdown = usageBreakdown(usage);
  if (!breakdown) return "暂无令牌用量";
  if (breakdown.hasBreakdown) {
    return `${breakdown.total} tokens (入 ${breakdown.input} · 出 ${breakdown.output})`;
  }
  return `${breakdown.total} tokens`;
}

export function promptCacheLabel(usage?: TokenUsage) {
  const count = usage?.prompt_cached_tokens ?? 0;
  if (!usage?.prompt_cache_reported && count <= 0) return "上游缓存未上报";
  if (usage?.prompt_cache_incomplete) return `上游缓存命中 ${count}（部分上报）`;
  return count > 0 ? `上游缓存命中 ${count}` : "上游缓存未命中（0）";
}

export function usageBreakdown(usage?: TokenUsage) {
  if (!usage || (usage.total_tokens === undefined && usage.prompt_tokens === undefined && usage.completion_tokens === undefined)) return null;
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
