/**
 * MyAI Mobile UI Prototype - Mock Files Data
 */

export const mockAssets = [
  {
    id: "asset-01",
    file_name: "android-ui-prototype.html",
    path: "/mobile/android-ui-prototype.html",
    size: 23393,
    content_type: "text/html",
    short_url: "https://assets.mikasa.wiki/s/proto-html",
    expires_at: 1774500000000
  },
  {
    id: "asset-02",
    file_name: "app-preview-screenshot.png",
    path: "/mobile/assets/preview.png",
    size: 142050,
    content_type: "image/png",
    short_url: "https://assets.mikasa.wiki/s/shot-png",
    expires_at: 1774500000000
  }
];

export const mockFileEntries = [
  { name: "..", is_dir: true, size: 0, mod_time: "" },
  { name: "components", is_dir: true, size: 0, mod_time: "2026-09-18 22:45" },
  { name: "constants", is_dir: true, size: 0, mod_time: "2026-09-18 22:40" },
  { name: "hooks", is_dir: true, size: 0, mod_time: "2026-09-18 22:42" },
  { name: "screens", is_dir: true, size: 0, mod_time: "2026-09-18 22:44" },
  { name: "types", is_dir: true, size: 0, mod_time: "2026-09-18 22:40" },
  { name: "protocol.ts", is_dir: false, size: 30872, mod_time: "2026-09-18 22:30" },
  { name: "App.tsx", is_dir: false, size: 697, mod_time: "2026-09-18 22:15" },
  { name: "package.json", is_dir: false, size: 1204, mod_time: "2026-09-18 22:00" },
  { name: "app.json", is_dir: false, size: 1536, mod_time: "2026-09-18 21:50" }
];

export const mockFilePreview = {
  path: "d:/Go_All/myai/mobile/src/protocol.ts",
  content: `export type PermissionState = {
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
};

export type SessionPermissionMode = "readonly" | "ask" | "full";
export type SessionAgentMode = "chat" | "plan";

export type ViewMode = "chat" | "knowledge" | "files" | "changes" | "changeDetail" | "sessions" | "settings" | "plan";

export type AgentRunSnapshot = {
  run_id: string;
  title: string;
  state: "queued" | "running" | "completed" | "failed" | "canceled";
  steps: Array<{ id: string; title: string; status: string }>;
};`,
  size: 30872,
  is_binary: false
};
