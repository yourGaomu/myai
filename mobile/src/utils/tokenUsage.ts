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

export function usageSummary(usage: TokenUsage) {
  if (usage.total_tokens !== undefined) {
    return `${usage.total_tokens} tokens`;
  }
  if (usage.prompt_tokens !== undefined || usage.completion_tokens !== undefined) {
    return `${tokenCount(usage.prompt_tokens)} in / ${tokenCount(usage.completion_tokens)} out`;
  }
  return "Usage unavailable";
}
