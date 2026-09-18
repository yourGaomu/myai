/**
 * MyAI Mobile UI Prototype - Mock Settings Data
 */

export const mockRelaySettings = {
  relayURL: "https://relay.mikasa.wiki",
  assetBaseURL: "https://assets.mikasa.wiki",
  bindCode: "682914",
  clientToken: "tok_mobile_8a7d9f2c1b",
  deviceID: "pixel-9-pro-dev",
  userID: "gaomu",
  connected: true,
  status: "Connected"
};

export const mockModels = [
  {
    id: "gemini-2.5-pro",
    name: "Gemini 2.5 Pro (Google)",
    protocol: "google-generative-ai",
    enabled: true,
    is_default: true,
    context_window: 128000
  },
  {
    id: "claude-3-7-sonnet",
    name: "Claude 3.7 Sonnet (Anthropic)",
    protocol: "anthropic-messages",
    enabled: true,
    is_default: false,
    context_window: 200000
  },
  {
    id: "deepseek-r1",
    name: "DeepSeek R1 (OpenAI 兼容)",
    protocol: "openai-chat-completions",
    enabled: true,
    is_default: false,
    context_window: 64000
  }
];

export const mockSkills = [
  { id: "agy-customizations", name: "Antigravity Customization", desc: "技能与插件开发规范" },
  { id: "generative-ui", name: "Generative UI Widget", desc: "富交互 HTML 渲染" }
];

export const mockPlugins = [
  { id: "plugin-bash", name: "Local Bash Terminal", enabled: true, desc: "终端命令执行器" },
  { id: "plugin-fs", name: "Workspace File Explorer", enabled: true, desc: "本地工作区文件操作" }
];

export const mockSubagents = [
  {
    id: "sub-01",
    name: "Code Reviewer",
    role: "代码审计子智能体",
    status: "idle",
    active_tasks: 0
  },
  {
    id: "sub-02",
    name: "UI Refactor Bot",
    role: "前端原型与样式构建",
    status: "running",
    active_tasks: 1
  }
];
