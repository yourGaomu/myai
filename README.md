# 我的readme

`myai` is our tiny Go CLI project. The long-term goal is to grow it into a coding assistant CLI.

## Run

```powershell
go run . help
go run . chat
```

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
