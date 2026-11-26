const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const { test } = require("node:test");
const ts = require("typescript");

// Run the production reducers and actions offline, with ref/state slots kept
// across simulated renders. The test controls when each frame is delivered.
require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  });
  module._compile(outputText, filename);
};
let nextSlot;
const load = Module._load;
Module._load = function (name, ...args) {
  if (name !== "react") return load.call(this, name, ...args);
  return {
    useCallback(fn) { return fn; },
    useMemo(fn) { return fn(); },
    useRef(initial) { return nextSlot(() => ({ current: initial })); },
    useState(initial) {
      const state = nextSlot(() => ({ value: initial }));
      return [state.value, (update) => { state.value = typeof update === "function" ? update(state.value) : update; }];
    },
  };
};
let useChatMessages, getSubmittedQuestions, useChatActions, latestRegeneratableTurnID, useRemoteMessageHandler, useSubagentState;
try {
  ({ useChatMessages, getSubmittedQuestions } = require("../src/hooks/useChatMessages.ts"));
  ({ useChatActions, latestRegeneratableTurnID } = require("../src/hooks/useChatActions.ts"));
  ({ useRemoteMessageHandler } = require("../src/hooks/useRemoteMessageHandler.ts"));
  ({ useSubagentState } = require("../src/hooks/useSubagentState.ts"));
} finally {
  Module._load = load;
}
const { buildChatTurns } = require("../src/utils/chatRenderItems.ts");
const { historyMessageToChatItem } = require("../src/utils/chatHistory.ts");
const { formatCacheRate, promptCacheLabel, promptCacheRate, turnUsageSummary, usageBreakdown } = require("../src/utils/tokenUsage.ts");
const { findSubagentForLaunch, subagentLaunchState } = require("../src/utils/subagentLaunch.ts");
const { toolKind } = require("../src/utils/toolDisplay.ts");

function harness() {
  const slots = [];
  const activeRequestIDRef = { current: "" };
  const requestSessionMapRef = { current: {} };
  const relayScopeRef = { current: { pairingID: "pair", connectionID: "c1" } };
  const sessionIDRef = { current: "s" };
  const events = [];
  const sent = [];
  const runs = [];
  const questionQueries = [];
  let accepting = true;
  function render() {
    let index = 0;
    nextSlot = (init) => {
      if (index === slots.length) slots.push(init());
      return slots[index++];
    };
    try {
      const state = useChatMessages();
      const sessionID = sessionIDRef.current;
      const sendEnvelope = (type, envelope) => {
        sent.push({ type, ...envelope });
        return accepting;
      };
      const actions = useChatActions({
        ...state, sessionID, sessionIDRef, activeRequestIDRef, requestSessionMapRef, relayScopeRef, sendEnvelope,
        addEventMessage: (...args) => events.push(args),
        addUserMessage: (session, text, request, attachments) => state.addMessage(session, "user", text, request, attachments),
        attachedFiles: [],
        getSessionRuns: () => runs,
        messageInput: "New input",
        pendingPermission: state.getSessionChat(sessionID).pendingPermission,
        requestUserQuestionSync: () => questionQueries.push(getSubmittedQuestions(state.getSessionChats())),
        sendMessageWithFiles: (content, requestID) => sendEnvelope("user_message", { request_id: requestID, session_id: sessionID, payload: { content } }),
        startPausePending() {}, stopPausePending() {},
      });
      const remote = useRemoteMessageHandler(new Proxy({
        ...state, activeRequestIDRef, requestSessionMapRef, relayScopeRef, sessionIDRef,
        acceptAgentRunList: () => true,
        applyRunList: (payload) => runs.splice(0, runs.length, ...payload.runs),
      }, { get: (target, key) => target[key] ?? (() => undefined) }));
      const subagents = useSubagentState(sessionID);
      return { ...state, ...actions, remote, subagents };
    } finally {
      nextSlot = undefined;
    }
  }
  return { render, activeRequestIDRef, requestSessionMapRef, relayScopeRef, sessionIDRef, events, sent, runs, questionQueries,
    setAccepting(value) { accepting = value; } };
}

const stored = (id, role, text) => historyMessageToChatItem({
  id, role, content: text, created_at: "2025-01-01T00:00:00Z",
});
const permission = (approvalID, requestID, extra = {}) => ({ approvalID, requestID, sessionID: "s", name: "tool", permission: "write", arguments: "{}", ...extra });
const question = (questionID, requestID, sessionID = "s") => ({
  questionID, requestID, sessionID, title: "Confirm",
  questions: [{ id: "details", prompt: "Details?", multi_select: false, allow_other: true, choices: [] }],
});
const turnID = (chat) => latestRegeneratableTurnID(buildChatTurns(chat.messages));

function addTurn(hook, requestID, status = "paused") {
  hook.addMessage("s", "user", `User ${requestID}`, requestID);
  hook.appendAssistant("s", requestID, `Partial ${requestID}`);
  hook.completeAssistant("s", requestID, status);
}

test("a cache read finishing after send or a delta preserves the optimistic and streaming rows", () => {
  const chat = harness().render();
  chat.addMessage("s", "user", "Live prompt", "live");
  chat.setSessionPendingRequest("s", "live");
  chat.appendAssistant("s", "live", "First");
  const activeID = chat.getSessionChat("s").activeAssistantID;
  chat.mergeSessionHistory("s", [stored("old", "user", "Older prompt")], "cache");
  assert.equal(chat.getSessionChat("s").activeAssistantID, activeID);
  chat.appendAssistant("s", "live", " second");
  assert.deepEqual(chat.getSessionChat("s").messages.map(({ text }) => text), ["Older prompt", "Live prompt", "First second"]);
  assert.equal(chat.getSessionChat("s").pendingRequestID, "live");
  chat.mergeSessionHistory("s", [], "server");
  chat.mergeSessionHistory("s", [stored("stale", "user", "Old cache")], "cache");
  assert.deepEqual(chat.getSessionChat("s").messages.map(({ text }) => text), ["Live prompt", "First second"]);
});

test("a terminal run settles only its request and paused content survives tool calls and late frames", () => {
  const chat = harness().render();
  chat.appendAssistant("s", "old", "Partial text", "Partial thought");
  chat.addToolCall("s", "tool", "{}", "old", "t1");
  chat.setSessionPendingRequest("s", "new");
  chat.appendAssistant("s", "new", "New stream");
  const activeID = chat.getSessionChat("s").activeAssistantID;
  chat.settleSessionRun("s", "old", "paused");
  chat.completeAssistant("s", "old", "paused");
  chat.appendAssistant("s", "old", "late chunk");
  const state = chat.getSessionChat("s");
  assert.equal(state.activeAssistantID, activeID);
  assert.equal(state.pendingRequestID, "new");
  assert.deepEqual(state.messages.map(({ text, reasoning, status }) => [text, reasoning, status]), [
    ["Partial text", "Partial thought", "paused"], ["", undefined, "done"], ["New stream", "", "streaming"],
  ]);
  chat.settleSessionRun("s", "new", "done");
  assert.equal(chat.getSessionChat("s").pendingRequestID, "");
  assert.equal(chat.getSessionChat("s").activeAssistantID, "");
});

test("assistant reply identity reconciles when authoritative history arrives before assistant_done", () => {
  const chat = harness().render();
  chat.addMessage("s", "user", "Optimistic", "r");
  chat.setSessionPendingRequest("s", "r");
  chat.appendAssistant("s", "r", "Par");
  chat.mergeSessionHistory("s", [stored("reply", "assistant", "Parsed")], "server");
  chat.completeAssistant("s", "r", "done", null, "Parsed", undefined, "reply");
  const messages = chat.getSessionChat("s").messages;
  assert.deepEqual(messages.map((item) => item.role), ["user", "assistant"]);
  const replies = messages.filter((item) => item.role === "assistant");
  assert.equal(replies.length, 1);
  assert.equal(replies[0].text, "Parsed");
  assert.equal(replies[0].historyID, "reply");
  assert.equal(replies[0].requestID, "r");
});

test("turn cache labels distinguish a hit, reported zero, unknown, and partial reporting", () => {
  assert.equal(promptCacheLabel({ prompt_cached_tokens: 80 }), "上游缓存命中 80");
  assert.equal(promptCacheLabel({ prompt_cache_reported: true }), "上游缓存未命中（0）");
  assert.equal(promptCacheLabel({ prompt_cached_tokens: 0 }), "上游缓存未上报");
  assert.equal(promptCacheLabel({ prompt_cache_incomplete: true }), "上游缓存未上报");
  assert.equal(promptCacheLabel(undefined), "上游缓存未上报");
  assert.equal(promptCacheLabel({ prompt_cache_reported: true, prompt_cache_incomplete: true }), "上游缓存命中 0（部分上报）");
  assert.equal(promptCacheLabel({ prompt_cached_tokens: 12, prompt_cache_incomplete: true }), "上游缓存命中 12（部分上报）");
  assert.equal(usageBreakdown({ prompt_cache_reported: true }), null);
});

test("turn usage summary keeps exact counts and does not treat missing counts as zero", () => {
  assert.equal(
    turnUsageSummary({ total_tokens: 1234 }),
    "输入 未上报 · 输出 未上报 · 上游缓存未上报 · 合计 1234 tokens",
  );
  assert.equal(
    turnUsageSummary({ prompt_tokens: 1200, completion_tokens: 0, prompt_cache_reported: true }),
    "输入 1200 · 输出 0 · 上游缓存未命中（0） · 合计 1200 tokens",
  );
  assert.equal(
    turnUsageSummary({ prompt_tokens: 1200, prompt_cached_tokens: 12, prompt_cache_incomplete: true }),
    "输入 1200 · 输出 未上报 · 上游缓存命中 12（部分上报） · 合计 未上报",
  );
  assert.equal(turnUsageSummary(), "输入 未上报 · 输出 未上报 · 上游缓存未上报 · 合计 未上报");
});

test("cache rate uses recorded input and refuses partial, legacy, or inconsistent counts", () => {
  const rate = (prompt, cached, flags = {}) => promptCacheRate({
    prompt_tokens: prompt, prompt_cached_tokens: cached, prompt_cache_reported: true, ...flags,
  });
  assert.equal(formatCacheRate(rate(15334, 15104)), "98.5%");
  assert.equal(formatCacheRate(rate(1200, undefined)), "0%");
  assert.equal(formatCacheRate(rate(250, 200)), "80%");
  assert.equal(formatCacheRate(rate(100, 100)), "100%");
  assert.equal(formatCacheRate(rate(10000, 1)), "<0.1%");
  assert.equal(formatCacheRate(rate(10000, 9999)), ">99.9%");
  assert.equal(rate(1200, 12, { prompt_cache_incomplete: true }), undefined);
  assert.equal(rate(1200, 12, { prompt_cache_reported: undefined }), undefined);
  for (const [input, cached] of [[undefined, 1], [0, 0], [10, -1], [10, 11], [10, 1.5], [Infinity, 1]]) {
    assert.equal(rate(input, cached), undefined);
  }
  // Session totals are weighted by tokens, even when turns have different input sizes.
  assert.equal(formatCacheRate(rate(100 + 900, 50 + 0)), "5%");
});

test("the turn footer gets all token counts from assistant_done and history", () => {
  const chat = harness().render();
  chat.addMessage("s", "user", "Question", "r");
  chat.appendAssistant("s", "r", "Answer");
  const usage = { prompt_tokens: 15334, completion_tokens: 5, total_tokens: 15339, prompt_cached_tokens: 15104 };
  const reply = { content: "Answer", usage };
  chat.remote({ type: "assistant_done", session_id: "s", request_id: "r", payload: { ...reply, message_id: "reply" } });
  const footer = (state) => turnUsageSummary(agentTurns(state.getSessionChat("s").messages)[0].usage);
  const expected = "输入 15334 · 输出 5 · 上游缓存命中 15104 · 合计 15339 tokens";
  assert.equal(footer(chat), expected);

  const reopened = harness().render();
  reopened.mergeSessionHistory("s", [historyMessageToChatItem({ ...reply, id: "reply", role: "assistant" })], "server");
  assert.equal(footer(reopened), expected);
});

test("history reconciliation preserves live cache usage until the matching row contains usage", () => {
  const chat = harness().render();
  chat.addMessage("s", "user", "Question", "r");
  chat.appendAssistant("s", "r", "Answer");
  const liveUsage = { prompt_cache_reported: true, prompt_cache_incomplete: true };
  chat.completeAssistant("s", "r", "done", liveUsage, "Answer", undefined, "reply");
  const history = (usage) => historyMessageToChatItem({ id: "reply", role: "assistant", content: "Answer", usage });
  chat.mergeSessionHistory("s", [history({})], "server");
  assert.deepEqual(agentTurns(chat.getSessionChat("s").messages)[0].usage, liveUsage);
  const storedUsage = { prompt_cached_tokens: 7, prompt_cache_reported: true };
  chat.mergeSessionHistory("s", [history(storedUsage)], "server");
  assert.deepEqual(agentTurns(chat.getSessionChat("s").messages)[0].usage, storedUsage);
});

test("background replies merge with cached and server history in either order and on replays", () => {
  for (const historyFirst of [false, true]) {
    const chat = harness().render();
    const saved = stored("reply-bg", "assistant", "Background answer");
    const send = (kind, sequence, fields = {}) => chat.remote({ type: "background_turn_event", session_id: "s",
      payload: { turn_id: "bg", run_id: "run-bg", kind, sequence, ...fields } });
    send("started", 1);
    send("delta", 2, { content: "Background" });
    if (historyFirst) chat.mergeSessionHistory("s", [saved], "cache");
    send("completed", 3, { message_id: saved.id, content: saved.text });
    chat.mergeSessionHistory("s", [saved], "server");
    send("completed", 3, { message_id: saved.id, content: saved.text });
    send("delta", 4, { content: "late" });
    assert.deepEqual(chat.getSessionChat("s").messages.map(({ historyID, requestID, text, status }) =>
      [historyID, requestID, text, status]), [[saved.id, "bg", saved.text, "done"]]);
  }
});

function runSnapshot(runID, requestID, replyID, content, extraEvents = []) {
  return {
    run: { id: runID, request_id: requestID, session_id: "s", kind: "chat", status: "succeeded",
      started_at: "2025-01-01T00:00:00Z", finished_at: "2025-01-01T00:00:01Z" },
    events: [...extraEvents, { id: `answer-${runID}`, type: "answer", message_id: replyID, content, sequence: 100 }],
  };
}

const agentTurns = (messages, runs) => buildChatTurns(messages, runs).filter((item) => item.type === "agent_turn");

test("a Run snapshot recovers a lost background completion without settling another turn", () => {
  const h = harness();
  const chat = h.render();
  chat.startBackgroundAssistant("s", "bg");
  chat.appendAssistant("s", "bg", "Partial");
  chat.setSessionPendingRequest("s", "foreground");
  chat.appendAssistant("s", "foreground", "In progress");
  chat.mergeSessionHistory("s", [stored("reply-bg", "assistant", "Finished")], "server");
  const run = runSnapshot("run-bg", "bg", "reply-bg", "Finished");
  chat.remote({ type: "agent_run_list_result", payload: { session_id: "s", runs: [run] } });
  const state = chat.getSessionChat("s");
  assert.equal(state.pendingRequestID, "foreground");
  assert.deepEqual(state.messages.map(({ requestID, status, text }) => [requestID, status, text]), [
    ["bg", "done", "Finished"], ["foreground", "streaming", "In progress"],
  ]);
  assert.deepEqual(agentTurns(state.messages, h.runs).map(({ text }) => text), ["Finished", "In progress"]);
});

test("two committed same-text replies keep separate cards and their own tools", () => {
  const chat = harness().render();
  const history = [];
  const runs = [];
  for (const key of ["foreground", "background"]) {
    const call = `call-${key}`;
    const reply = `reply-${key}`;
    chat.addToolCall("s", "spawn_agent", "{}", key, call);
    chat.addToolResult("s", "spawn_agent", "{}", "ok", false, key, { status: "success" }, call);
    chat.completeAssistant("s", key, "done", null, "Same answer", undefined, reply);
    history.push({ ...stored(call, "tool_call", ""), toolName: "spawn_agent", toolCallID: call },
      { ...stored(`result-${key}`, "tool", "ok"), toolName: "spawn_agent", toolCallID: call },
      stored(reply, "assistant", "Same answer"));
    runs.push(runSnapshot(`run-${key}`, key, reply, "Same answer", [
      { id: `event-${key}`, type: "tool_call", tool_call_id: call, tool_name: "spawn_agent", sequence: 1 },
    ]));
  }
  chat.mergeSessionHistory("s", history, "server");
  assert.deepEqual(agentTurns(chat.getSessionChat("s").messages, runs).map(({ text, tools }) =>
    [text, tools.map((tool) => tool.toolCallID)]), [
    ["Same answer", ["call-foreground"]], ["Same answer", ["call-background"]],
  ]);
  assert.equal(agentTurns(history, runs).length, 2);
});

test("tools cannot borrow another turn's answer or duplicate their own answer across an event", () => {
  const call = { id: "call", role: "tool_call", text: "", toolName: "spawn_agent", toolCallID: "call" };
  const notice = { id: "notice", role: "event", text: "Notice" };
  const reply = { ...stored("reply", "assistant", "Answer"), requestID: "r", status: "done" };
  const snapshot = runSnapshot("run", "r", "reply", "Answer");
  for (const requestID of [undefined, "r"]) {
    const turns = agentTurns([{ ...call, requestID }, notice, reply], [snapshot]);
    assert.deepEqual(turns.map(({ text, tools }) => [text, tools.length]), [["", 1], ["Answer", 0]]);
  }
});

test("parallel identical tool arguments do not cross-match or duplicate on late and replayed frames", () => {
  const chat = harness().render();
  chat.addToolCall("s", "spawn_agent", "{}", "r", "a");
  chat.addToolResult("s", "spawn_agent", "{}", "denied", true, "r", { status: "denied" }, "b");
  chat.addToolCall("s", "spawn_agent", "{}", "r", "b");
  chat.addToolCall("s", "spawn_agent", "{}", "r", "b");
  assert.equal(chat.getSessionChat("s").messages.find((item) => item.toolCallID === "a").status, "tool_running");
  chat.completeAssistant("s", "r", "done", null, "Answer", undefined, "reply");
  const snapshot = runSnapshot("run", "r", "reply", "Answer", [
    { id: "result-a", type: "tool_result", tool_call_id: "a", tool_name: "spawn_agent", status: "success", sequence: 1 },
  ]);
  const expected = [["a", "completed"], ["b", "error"]];
  const toolStatuses = (runs) => agentTurns(chat.getSessionChat("s").messages, runs)[0].tools.map(({ toolCallID, status }) => [toolCallID, status]);
  assert.deepEqual(toolStatuses([snapshot]), expected);
  chat.addToolResult("s", "spawn_agent", "{}", "ok", false, "r", { status: "success" }, "a");
  chat.addToolResult("s", "spawn_agent", "{}", "ok", false, "r", { status: "success" }, "a");
  assert.deepEqual(toolStatuses([]), expected);
  assert.equal(chat.getSessionChat("s").messages.filter((item) => item.role === "tool").length, 2);
});

test("subagent snapshots settle the launch row without a tool result and reject stale or unrelated tasks", () => {
  const h = harness();
  const chat = h.render();
  const call = { id: "row", toolCallID: "call", name: "spawn_agent", status: "running" };
  const queued = { id: "task", parent_session_id: "s", parent_run_id: "run", parent_tool_call_id: "call",
    status: "queued", created_at: "2025-01-01T00:00:00Z", updated_at: "2025-01-01T00:00:01Z" };
  const running = { ...queued, status: "running", started_at: queued.updated_at };
  const done = { ...running, status: "succeeded" };
  const other = { ...queued, id: "other", parent_session_id: "child" };
  const historical = { ...queued, id: "old-task", parent_run_id: "old-run" };
  assert.equal(findSubagentForLaunch(call, [other, historical, done], "s", "run"), done);
  assert.equal(findSubagentForLaunch(call, [historical, done], "s"), undefined);
  assert.equal(subagentLaunchState(call, undefined, true), "starting");
  assert.equal(subagentLaunchState(call, queued, true), "queued");
  assert.equal(subagentLaunchState(call, running, true), "started");
  assert.equal(subagentLaunchState(call, done, true), "started");
  assert.equal(subagentLaunchState(call, { ...running, status: "failed" }, false), "started");
  assert.equal(subagentLaunchState({ ...call, status: "error" }, done, true), "failed");
  assert.equal(subagentLaunchState({ ...call, status: "completed", toolStatus: "skipped" }, undefined, false), "skipped");
  assert.equal(subagentLaunchState(call, undefined, false), "unconfirmed");
  assert.equal(toolKind("list_subagent_definitions"), "list");
  chat.subagents.applyTaskResult({ task: done, kind: "task.completed", sequence: 3 });
  chat.subagents.applyTaskResult({ task: running });
  chat.subagents.applyTaskList({ session_id: "s", tasks: [running] });
  chat.subagents.applyTaskResult({ task: running, kind: "task.started", sequence: 2 });
  assert.equal(h.render().subagents.tasks[0].status, "succeeded");
});

test("approval replays preserve fresh consent and only the exact approval or its terminal turn clears it", () => {
  const h = harness();
  const chat = h.render();
  chat.setSessionPendingPermission("s", permission("a", "run-a"));
  chat.setSessionPendingPermission("s", permission("b", "run-a"));
  chat.setSessionPendingPermission("s", permission("c", "run-c"));
  const actions = h.render();
  chat.setSessionPendingPermission("s", permission("a", "run-a", { requireFreshConsent: true }));
  chat.setSessionPendingPermission("s", permission("a", "run-a"));
  actions.allowPermissionSession();
  assert.deepEqual(h.sent, []);
  actions.allowPermission();
  assert.equal(h.sent[0].payload.scope, "once");
  assert.equal(chat.getSessionChat("s").pendingPermission.approvalID, "a");
  chat.removeSessionPendingPermission("s", "run-c", "a");
  chat.removeSessionPendingPermission("s", "run-a", "b");
  assert.deepEqual(chat.getSessionChat("s").pendingPermissions.map(({ approvalID }) => approvalID), ["a", "c"]);
  chat.removeSessionPendingPermission("s", "run-a");
  actions.denyPermission();
  assert.equal(h.sent.length, 1);
  assert.equal(chat.getSessionChat("s").pendingPermission.approvalID, "c");
});

test("a lost question submission can retry with its draft, while stale callbacks and identities cannot answer", () => {
  const h = harness();
  const chat = h.render();
  const prompt = question("q1", "run-a");
  chat.setSessionPendingQuestion("s", prompt);
  const drafts = { details: { selected: [], other: "My input" } };
  const structuredAnswers = { details: { other: "My input" } };
  const submissionPayload = { structuredAnswers };
  const skipPayload = { structuredAnswers: {} };
  chat.setSessionQuestionDraft(prompt, drafts);
  assert.equal(chat.answerUserQuestion(prompt, submissionPayload), true);
  const submission = chat.getSessionChat("s").pendingQuestions[0];
  assert.equal(h.questionQueries[0].get("q1"), submission);
  assert.equal(chat.answerUserQuestion(prompt, submissionPayload), false);
  assert.equal(chat.answerUserQuestion(question("q1", "other"), skipPayload, true), false);
  chat.removeSessionPendingQuestion("s", "q1", "other");
  h.relayScopeRef.current = { pairingID: "pair", connectionID: "c2" };
  chat.setSessionPendingQuestion("s", { ...prompt, submitted: false });
  assert.deepEqual(chat.getSessionChat("s").pendingQuestions[0].draftAnswers, drafts);
  assert.equal(chat.answerUserQuestion(prompt, skipPayload, true), false);
  const reconnected = h.render();
  assert.equal(reconnected.answerUserQuestion(prompt, submissionPayload), true);
  assert.notEqual(h.questionQueries[1].get("q1"), submission);
  for (const { payload } of h.sent) {
    assert.equal(payload.schema_version, 2);
    assert.deepEqual(payload.structured_answers, structuredAnswers);
    assert.equal(Object.hasOwn(payload, "answers"), false);
  }
});

test("regenerate rejects old turns, double taps, live runs and stale session callbacks", () => {
  const h = harness();
  const chat = h.render();
  addTurn(chat, "old");
  const old = turnID(chat.getSessionChat("s"));
  addTurn(chat, "latest");
  const latest = turnID(chat.getSessionChat("s"));
  assert.ok(old && latest && old !== latest);
  chat.regenerateSession(old);
  h.runs.push({ run: { id: "live", status: "running", started_at: "2025-01-01" }, events: [] });
  chat.regenerateSession(latest);
  h.runs.length = 0;
  h.sessionIDRef.current = "other";
  chat.regenerateSession(latest);
  assert.equal(h.sent.length, 0);
  h.sessionIDRef.current = "s";
  chat.regenerateSession(latest);
  chat.regenerateSession(latest);
  assert.deepEqual(h.sent.map(({ type }) => type), ["session_regenerate"]);
  assert.equal(chat.getSessionChat("s").pendingRequestID, h.sent[0].request_id);
});

test("an unanswered newer input rules out regenerating the older paused reply", () => {
  const chat = harness().render();
  addTurn(chat, "old");
  chat.addMessage("s", "user", "Newer input", "new");
  assert.equal(turnID(chat.getSessionChat("s")), undefined);
});


test("Jev updates one card for its own request and ignores a stale started snapshot", () => {
  const h = harness();
  const chat = h.render();
  h.requestSessionMapRef.current.r = "s";
  const trace = {
    trace_id: "trace", session_id: "s", request_id: "r",
    created_at: "2026-11-27T09:00:00Z", status: "started", request_body: '{"state":{"latest_request":"test"}}',
  };
  const event = { type: "intent_trace_event", request_id: "r", session_id: "s", payload: trace };
  chat.remote(event);
  assert.equal(chat.getSessionChat("s").messages[0]?.intentTrace?.status, "started");
  chat.remote({ ...event, payload: { ...trace, trace_id: "wrong", session_id: "other" } });
  chat.remote({ ...event, payload: { ...trace, status: "succeeded", route: "chat", response_body: '{"answer":"conversation"}' } });
  chat.upsertIntentTrace("s", { ...trace, request_body: undefined }, true);
  const messages = chat.getSessionChat("s").messages;
  assert.equal(messages.length, 1);
  assert.equal(messages[0].intentTrace.status, "succeeded");
  assert.equal(messages[0].intentTrace.request_body, trace.request_body);
  assert.equal(messages[0].intentTrace.response_body, '{"answer":"conversation"}');
  assert.equal(buildChatTurns(messages, [])[0].type, "event");
});
