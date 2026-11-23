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

export function promptCacheRate(usage?: TokenUsage) {
  if (!usage?.prompt_cache_reported || usage.prompt_cache_incomplete) return undefined;
  const input = usage.prompt_tokens;
  const cached = usage.prompt_cached_tokens ?? 0;
  if (input === undefined || !Number.isSafeInteger(input) || input <= 0 ||
      !Number.isSafeInteger(cached) || cached < 0 || cached > input) return undefined;
  return cached / input;
}

export function formatCacheRate(rate: number) {
  const percent = rate * 100;
  if (percent > 0 && percent < 0.1) return "<0.1%";
  if (percent > 99.9 && percent < 100) return ">99.9%";
  return `${Number(percent.toFixed(1))}%`;
}

export function turnUsageCounts(usage?: TokenUsage) {
  const input = usage?.prompt_tokens;
  const output = usage?.completion_tokens;
  const total = usage?.total_tokens ?? (input !== undefined && output !== undefined ? input + output : undefined);
  return { input, output, total };
}

export function turnUsageSummary(usage?: TokenUsage) {
  const { input, output, total } = turnUsageCounts(usage);
  return [
    `输入 ${input ?? "未上报"}`,
    `输出 ${output ?? "未上报"}`,
    promptCacheLabel(usage),
    `合计 ${total === undefined ? "未上报" : `${total} tokens`}`,
  ].join(" · ");
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
