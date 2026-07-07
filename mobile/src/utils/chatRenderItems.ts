import type { ChatItem } from "../types/chat";
import { parseSharedAsset } from "./toolAssets";

export type ToolActivityGroupItem = {
  id: string;
  messages: ChatItem[];
  names: string[];
  callCount: number;
  resultCount: number;
  failedCount: number;
  permissionCount: number;
  assetCount: number;
};

export type ChatRenderItem =
  | { type: "message"; id: string; message: ChatItem }
  | { type: "tool_group"; id: string; group: ToolActivityGroupItem };

export function groupToolActivity(messages: ChatItem[]): ChatRenderItem[] {
  const items: ChatRenderItem[] = [];
  let group: ChatItem[] = [];

  const flushGroup = () => {
    if (group.length === 0) {
      return;
    }

    const first = group[0];
    const last = group[group.length - 1];
    const names = uniqueToolNames(group);
    const failedCount = group.filter((message) => Boolean(message.toolError)).length;
    const callCount = group.filter((message) => message.role === "tool_call").length;
    const resultCount = group.filter((message) => message.role === "tool").length;
    const permissionCount = group.filter(isPermissionEvent).length;
    const assetCount = group.filter((message) => Boolean(parseSharedAsset(message.toolName, message.text))).length;

    items.push({
      type: "tool_group",
      id: `tool-group-${first.id}-${last.id}`,
      group: {
        id: `tool-group-${first.id}-${last.id}`,
        messages: group,
        names,
        callCount,
        resultCount,
        failedCount,
        permissionCount,
        assetCount,
      },
    });
    group = [];
  };

  messages.forEach((message) => {
    if (isToolActivityMessage(message)) {
      group.push(message);
      return;
    }

    flushGroup();
    items.push({ type: "message", id: message.id, message });
  });

  flushGroup();
  return items;
}

export function isPermissionEvent(message: ChatItem) {
  return message.role === "event" && /^(Allowed|Denied)\s+\S+/.test(message.text.trim());
}

function isToolActivityMessage(message: ChatItem) {
  return message.role === "tool_call" || message.role === "tool" || isPermissionEvent(message);
}

function uniqueToolNames(messages: ChatItem[]) {
  const names: string[] = [];
  messages.forEach((message) => {
    const name = message.toolName?.trim();
    if (name && !names.includes(name)) {
      names.push(name);
    }
  });
  return names;
}
