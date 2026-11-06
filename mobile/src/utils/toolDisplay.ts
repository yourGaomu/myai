import type { ComponentProps } from "react";
import type { Feather } from "@expo/vector-icons";

// 把工具调用翻译成界面上的「动词 + 对象」，供对话时间线和权限确认共用。

export type ToolKind =
  | "shell"
  | "read"
  | "write"
  | "edit"
  | "search"
  | "list"
  | "web"
  | "knowledge"
  | "subagent_wait"
  | "subagent_spawn"
  | "other";

type FeatherName = ComponentProps<typeof Feather>["name"];

export function toolKind(rawName: string): ToolKind {
  const name = rawName.toLowerCase();
  if (name === "wait_agent") return "subagent_wait";
  if (name === "spawn_agent" || name === "start_async_task" || name.includes("subagent")) return "subagent_spawn";
  if (name.includes("knowledge")) return "knowledge";
  if (/(bash|shell|powershell|terminal|command|exec)|(^|_)(cmd|sh)($|_)/.test(name)) return "shell";
  if (/(fetch|http|browse|url|web)/.test(name)) return "web";
  if (/(grep|search|find|glob)/.test(name)) return "search";
  if (/(list|ls|tree)/.test(name)) return "list";
  if (/(write|create)/.test(name)) return "write";
  if (/(edit|replace|patch)/.test(name)) return "edit";
  if (/(read|view|cat|open)/.test(name)) return "read";
  return "other";
}

const icons: Record<ToolKind, FeatherName> = {
  shell: "terminal",
  read: "file-text",
  write: "file-plus",
  edit: "edit-3",
  search: "search",
  list: "folder",
  web: "globe",
  knowledge: "book-open",
  subagent_wait: "clock",
  subagent_spawn: "cpu",
  other: "tool",
};

const verbs: Record<ToolKind, [running: string, done: string]> = {
  shell: ["正在运行", "已运行"],
  read: ["正在读取", "已读取"],
  write: ["正在写入", "已创建"],
  edit: ["正在修改", "已修改"],
  search: ["正在搜索", "已搜索"],
  list: ["正在列出", "已列出"],
  web: ["正在访问", "已访问"],
  knowledge: ["正在检索知识库", "已检索知识库"],
  subagent_wait: ["正在等待子代理", "已获取子代理结果"],
  subagent_spawn: ["正在启动子代理", "已启动子代理"],
  other: ["正在调用", "已调用"],
};

export function toolIcon(kind: ToolKind): FeatherName {
  return icons[kind];
}

export function toolVerb(kind: ToolKind, status: "running" | "done" | "error") {
  if (status === "error") {
    return `${verbs[kind][1].replace(/^已/, "")}失败`;
  }
  return status === "running" ? verbs[kind][0] : verbs[kind][1];
}

export function parseToolArgs(raw: unknown): Record<string, unknown> | string | undefined {
  if (raw === undefined || raw === null) return undefined;
  if (typeof raw === "object") return raw as Record<string, unknown>;
  const text = String(raw).trim();
  if (!text) return undefined;
  try {
    const parsed = JSON.parse(text);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) return parsed as Record<string, unknown>;
    if (typeof parsed === "string") return parsed;
  } catch {
    // 非 JSON 参数按原文处理
  }
  return text.startsWith("{") || text.startsWith("[") ? undefined : text;
}

function pick(args: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = args[key];
    if (value !== undefined && value !== null && typeof value !== "object" && String(value).trim()) {
      return String(value).trim();
    }
  }
  return "";
}

const commandKeys = ["command", "cmd", "CommandLine", "command_line", "script"];
const pathKeys = ["path", "file_path", "TargetFile", "AbsolutePath", "file", "filename"];
const queryKeys = ["query", "Query", "pattern", "Pattern", "regex"];
const urlKeys = ["url", "Url", "URL"];
const cwdKeys = ["cwd", "workdir", "working_directory", "Cwd", "dir"];

export function toolCommand(raw: unknown) {
  const args = parseToolArgs(raw);
  if (typeof args === "string") return args;
  return args ? pick(args, commandKeys) : "";
}

export function toolPath(raw: unknown) {
  const args = parseToolArgs(raw);
  return args && typeof args === "object" ? pick(args, pathKeys) : "";
}

export function toolCwd(raw: unknown) {
  const args = parseToolArgs(raw);
  return args && typeof args === "object" ? pick(args, cwdKeys) : "";
}

export function baseName(path: string) {
  return path.split(/[/\\]/).filter(Boolean).pop() || path;
}

export function dirName(path: string) {
  const parts = path.split(/[/\\]/).filter(Boolean);
  return parts.length > 1 ? parts.slice(0, -1).join("/") : "";
}

// 时间线一行里「动词」后面的对象：命令、文件名、搜索词、网址等。
export function toolTarget(name: string, raw: unknown) {
  const kind = toolKind(name);
  const args = parseToolArgs(raw);
  if (typeof args === "string") return args.replace(/\s+/g, " ");
  if (!args) return name;
  let value = "";
  if (kind === "shell") value = pick(args, commandKeys);
  else if (kind === "read" || kind === "write" || kind === "edit" || kind === "list") value = baseName(pick(args, pathKeys));
  else if (kind === "search" || kind === "knowledge") value = pick(args, queryKeys) || pick(args, pathKeys);
  else if (kind === "web") value = (pick(args, urlKeys).match(/^[a-z]+:\/\/([^/?#]+)/i) || [])[1] || pick(args, urlKeys);
  else if (kind === "subagent_wait" || kind === "subagent_spawn") value = pick(args, ["agent_nickname", "title", "task_id", "name"]);
  else value = pick(args, [...pathKeys, ...queryKeys, "name", "title"]);
  return (value || name).replace(/\s+/g, " ");
}

// 权限确认弹层的标题。
export function permissionTitle(name: string) {
  const kind = toolKind(name);
  if (kind === "shell") return "允许运行这条命令？";
  if (kind === "write" || kind === "edit") return "允许修改这个文件？";
  if (kind === "web") return "允许访问这个网址？";
  return `允许调用 ${name}？`;
}

// 权限确认弹层里展示的主体：命令原文、文件路径或格式化后的参数。
export function permissionBody(name: string, raw: unknown) {
  const kind = toolKind(name);
  const args = parseToolArgs(raw);
  if (typeof args === "string") return args;
  if (!args) return String(raw || "").trim();
  if (kind === "shell") return pick(args, commandKeys) || JSON.stringify(args, null, 2);
  if (kind === "write" || kind === "edit") return pick(args, pathKeys) || JSON.stringify(args, null, 2);
  if (kind === "web") return pick(args, urlKeys) || JSON.stringify(args, null, 2);
  return JSON.stringify(args, null, 2);
}
