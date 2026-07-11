# MyAI

MyAI 是一个 Go + Expo React Native 实现的 AI 编程助手，支持命令行聊天、手机远程控制、会话与模型管理、Plan 模式、工具调用、权限审批、文件预览和工作区变更恢复。

可先打开 [PROJECT_ARCHITECTURE_INTRO.html](PROJECT_ARCHITECTURE_INTRO.html) 查看动画导览，再阅读 [PROJECT_ARCHITECTURE_GUIDE.md](PROJECT_ARCHITECTURE_GUIDE.md) 了解完整架构、Spring Boot 对照和调用链。

## Run

```powershell
go run . help
go run . chat
```

## Remote Agent With Files And Changes

Start the relay:

```powershell
go run . relay --addr 0.0.0.0:18080
```

Start the PC agent and choose the workspace that clients can preview:

```powershell
go run . agent --server ws://127.0.0.1:18080/ws/agent --user local --device pc-local --workspace D:\Go_All\myai
```

After pairing the Android app, open `Files` to browse and preview files from that workspace, or open `Changes` to inspect changes compared with the SQLite workspace history baseline, preview diffs, and revert restorable files.

## Current Commands

- `myai help`: shows command help
- `myai chat`: starts a simple interactive chat loop

Inside chat:

- `/help`: shows chat commands
- `/exit`: leaves chat

## Main Capabilities

- OpenAI-compatible model providers and streaming responses
- Chat and Plan session modes
- Local tools, permission approval, Skills, Hooks, and MCP tools
- MongoDB persistence, Redis current-session cache, and SQLite workspace history
- Relay-based mobile pairing and remote Agent control
- Mobile session, file, change, model, context, and Plan management
