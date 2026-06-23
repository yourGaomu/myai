# MyAI Project Assistant

Triggers: myai, go cli, cobra, langchaingo, agent, relay, android, expo, skill, skillhub, mcp, 会话管理, 工具调用, 文件回显, 权限, 上下文压缩

Use this skill when helping develop the `myai` project, a Go coding assistant CLI with a remote agent, relay server, and Expo/React Native mobile client.

## Project Shape

- Keep the Go backend, relay protocol, agent runtime, local tools, storage, and mobile app as separate layers.
- Prefer extending existing modules instead of putting cross-cutting logic into one large file.
- For mobile work, keep screen composition in `mobile/src/screens`, stateful logic in `mobile/src/hooks`, reusable UI in `mobile/src/components`, and shared types/utilities in `mobile/src/types` or `mobile/src/utils`.
- For remote features, add protocol message types first, then agent handlers, then mobile request/result handling, then UI.

## Go Backend Rules

- Use `gofmt` after Go edits.
- Run focused `go test` for changed packages, and run `go test ./...` after protocol, session, tool, storage, or agent changes.
- Keep session-specific behavior keyed by session id.
- Do not store tool calls, tool results, reasoning, token usage, or permission decisions as plain assistant text. Preserve them as structured records when possible.
- File-changing behavior should go through local tools and history recording so diffs and rollback can work without relying on Git.

## Skill And MCP Direction

- Skills are local instructions loaded from `skills/<name>/SKILL.md`.
- A skill should be discovered by trigger terms, but full skill content should only be injected when the latest user request matches.
- Merge selected skill prompts into the system prompt for the model call. Do not append skill text into conversation history.
- Keep stable index/prefix text small so future prompt caching has a better chance of hitting.
- MCP should be added as a separate tool/provider layer later; do not mix MCP server lifecycle directly into chat UI code.

## Mobile UX Rules

- Android/mobile UI should expose connection, sessions, models, tools/skills, files, changes, token usage, and permissions clearly.
- Buttons that start network or agent work should show loading feedback and avoid freezing the full screen.
- Long chat content, diff views, file previews, and settings panels should scroll inside bounded areas.
- Multi-session behavior should always send `session_id` and render history for the selected session.

## Verification Checklist

- Go: `go test ./...`
- Mobile: `cd mobile && npm run typecheck`
- Skill hot reload: `go run . skill reload`
