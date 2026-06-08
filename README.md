# 我的AI项目

`myai` is our tiny Go CLI project. The long-term goal is to grow it into a coding assistant CLI.

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

After pairing the Android app, open `Files` to browse and preview files from that workspace, or open `Changes` to inspect Git changes and diffs.

## Current Commands

- `myai help`: shows command help
- `myai chat`: starts a simple interactive chat loop

Inside chat:

- `/help`: shows chat commands
- `/exit`: leaves chat

## What This Version Does

This first version does not call an AI model yet. It only proves that:

- the Go project is set up
- the CLI can parse a command
- the chat loop can read your input
- special commands like `/help` and `/exit` work
