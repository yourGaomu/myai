const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const { test } = require("node:test");
const ts = require("typescript");

require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  });
  module._compile(outputText, filename);
};

let fixture;
const originalLoad = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") {
    return {
      useCallback: (callback) => callback,
      useRef: (initial) => ({ current: initial }),
      useEffect: (setup) => { fixture.cleanups.push(setup()); },
    };
  }
  if (name === "../storage/sessionHistoryCache") {
    return {
      loadCachedSessionHistory: (key) => fixture.readCache(key),
      replaceCachedSessionHistory: async (key, messages) => { fixture.writes.push({ key, messages }); },
    };
  }
  return originalLoad.call(this, name, ...args);
};
let useRelaySender, useSessionHistorySync, useRemoteRequests;
try {
  ({ useRelaySender } = require("../src/hooks/useRelaySender.ts"));
  ({ useSessionHistorySync } = require("../src/hooks/useSessionHistorySync.ts"));
  ({ useRemoteRequests } = require("../src/hooks/useRemoteRequests.ts"));
} finally {
  Module._load = originalLoad;
}
const { normalizeRelayUrl, restoreRelaySettings } = require("../src/utils/relayPairing.ts");
const { startRelayWebSocket } = require("../src/utils/relayWebSocket.ts");
const { createMCPRequests } = require("../src/hooks/useMCPState.ts");

const noop = () => {};
const tick = () => new Promise((resolve) => setImmediate(resolve));
const pairing = {
  pairingID: "pairing-a", relayURL: "https://relay.example.invalid",
  userID: "test-user", deviceID: "test-agent", clientToken: "test-credential",
};
const identity = { user_id: pairing.userID, device_id: pairing.deviceID };

function socketFixture(t) {
  const previous = globalThis.WebSocket;
  const sockets = [];
  globalThis.WebSocket = class FakeWebSocket {
    static OPEN = 1;
    constructor(url) {
      this.url = url;
      this.readyState = 0;
      this.sent = [];
      this.closed = 0;
      sockets.push(this);
    }
    open() { this.readyState = 1; this.onopen?.(); }
    close() { this.readyState = 3; this.closed++; }
    send(data) { this.sent.push(JSON.parse(data)); }
    receive(message) { this.onmessage?.({ data: JSON.stringify(message) }); }
  };
  const available = [];
  const received = [];
  const losses = [];
  const close = startRelayWebSocket(pairing, {
    onAuthenticating: noop,
    onAuthenticated: (handle) => available.push(handle),
    onMessage: (message) => received.push(message),
    onLost: (loss) => losses.push(loss),
  });
  t.after(() => { close(); globalThis.WebSocket = previous; });
  return { socket: sockets[0], available, received, losses, close };
}

const heartbeatAck = (requestID, fields = {}) => ({
  ...identity,
  type: "heartbeat", request_id: requestID, payload: { received: "heartbeat" }, ...fields,
});

test("Relay URL migration preserves the issuer and rejects incomplete settings", () => {
  assert.equal(normalizeRelayUrl("WSS://Relay.Example.Invalid:443/ws/client/"), pairing.relayURL);
  assert.equal(normalizeRelayUrl("10.0.2.2:18080"), "http://10.0.2.2:18080");
  assert.equal(normalizeRelayUrl("https://relay.example.invalid:99999"), "");
  assert.equal(normalizeRelayUrl("https://relay.example.invalid/?x=y"), "");
  const restored = restoreRelaySettings({ ...pairing, pairingID: "", relayURL: "http://127.0.0.1:18080" });
  assert.equal(restored.pairing.relayURL, "http://127.0.0.1:18080");
  assert.ok(restored.pairing.pairingID);
  assert.equal(restoreRelaySettings({ ...pairing, relayURL: "" }).pairing, null);
  assert.equal(restoreRelaySettings({ ...pairing, deviceID: "" }).pairing, null);
});

test("a WebSocket authenticates only on its own heartbeat and then drains replay", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { socket, available, received, losses } = socketFixture(t);
  assert.equal(socket.url, "wss://relay.example.invalid/ws/client");
  socket.open();
  const first = socket.sent[0];
  assert.equal(first.type, "heartbeat");
  assert.equal(first.client_token, pairing.clientToken);
  socket.receive({ ...identity, type: "permission_ask", request_id: "existing-turn", client_token: "omit" });
  socket.receive(heartbeatAck("another-request"));
  socket.receive(heartbeatAck(first.request_id, { device_id: "different-device" }));
  socket.receive(heartbeatAck(first.request_id, { payload: { received: "session_list" } }));
  assert.deepEqual(available, []);
  assert.deepEqual(received, []);
  socket.receive(heartbeatAck(first.request_id));
  assert.equal(available.length, 1);
  assert.equal(available[0].pairing, pairing);
  assert.deepEqual(received, [{ ...identity, type: "permission_ask", request_id: "existing-turn" }]);

  t.mock.timers.tick(25_000);
  const next = socket.sent[1];
  assert.notEqual(next.request_id, first.request_id);
  socket.receive(heartbeatAck(first.request_id));
  t.mock.timers.tick(10_000);
  assert.deepEqual(losses.map((loss) => loss.state), ["reconnecting"]);
  assert.equal(available.length, 1);
});

test("typed pairing rejection and temporary storage failure stay offline with different retry states", (t) => {
  for (const [payload, expected] of [
    [{ error_code: "client_auth_rejected" }, "auth_rejected"],
    [{ error_code: "client_auth_unavailable" }, "reconnecting"],
    [{ message: "client token is invalid or expired" }, "reconnecting"],
  ]) {
    const { socket, available, received, losses, close } = socketFixture(t);
    socket.open();
    socket.receive({ ...identity, type: "error", request_id: socket.sent[0].request_id, payload });
    assert.deepEqual(losses.map((loss) => loss.state), [expected]);
    assert.equal(available.length, 0);
    assert.equal(received.length, 0);
    close();
  }
});

test("a retired socket closes on a late upgrade and sends no heartbeat", (t) => {
  const { socket, available, received, close } = socketFixture(t);
  close();
  socket.open();
  assert.equal(socket.closed, 2);
  assert.equal(socket.sent.length, 0);
  assert.equal(available.length, 0);
  assert.equal(received.length, 0);
});

test("a definitive native rejection reaches the original turn after disconnect, but never after re-pair", async () => {
  const socketRef = { current: null };
  const currentPairingRef = { current: pairing };
  const failures = [];
  const errors = [];
  const frames = [];
  const replies = [];
  socketRef.current = {
    pairing, connectionID: "socket-a",
    send: (raw) => { frames.push(JSON.parse(raw)); return new Promise((resolve) => replies.push(resolve)); },
  };
  const send = useRelaySender({
    socketRef, currentPairingRef, activeRequestIDRef: { current: "" }, sessionIDRef: { current: "session-a" },
    onEnqueueFailure: (frame) => failures.push(frame), addErrorMessage: (text) => errors.push(text),
  });
  assert.equal(send("user_message", { request_id: "chat-a" }), true);
  assert.equal(send("asset_list", { request_id: "asset-a" }), true);
  assert.equal(frames[0].user_id, pairing.userID);
  assert.equal(frames[0].session_id, "session-a");
  const old = socketRef.current;
  socketRef.current = null;
  replies[1](false);
  replies[0](false);
  await tick();
  assert.equal(failures.length, 1);
  assert.equal(failures[0].request_id, "chat-a");
  assert.equal(failures[0].payload.request_type, "user_message");
  assert.equal(failures[0].payload.transport_error, true);
  socketRef.current = old;
  assert.equal(send("user_message", { request_id: "chat-b" }), true);
  currentPairingRef.current = { ...pairing, pairingID: "pairing-b" };
  assert.equal(send("user_message"), false);
  replies[2](false);
  await tick();
  assert.equal(failures.length, 1);
  assert.equal(errors.length, 1);
});

function historyFixture(t, readCache = async () => ({ messages: [], meta: {} }), hook = useSessionHistorySync) {
  const args = {
    historySessionIDRef: { current: "" }, pendingHistorySessionIDRef: { current: "" },
    relayScopeRef: { current: { pairingID: "pairing-a", connectionID: "connection-a" } },
    sessionIDRef: { current: "session-a" },
  };
  const sent = [], merged = [], pending = [], writes = [], cleanups = [];
  fixture = { readCache, writes, cleanups };
  t.after(() => { for (const cleanup of cleanups) cleanup?.(); });
  const api = hook({
    ...args, clientToken: pairing.clientToken,
    sendEnvelope: (type, frame) => { sent.push({ type, ...frame }); return true; },
    mergeSessionHistory: (sessionID, messages, source) => merged.push({ sessionID, messages, source }),
    startPending: (group) => pending.push([group, true]), stopPending: (group) => pending.push([group, false]),
  });
  const frame = (request, payload) => ({
    type: `${request.type}_result`, request_id: request.request_id, session_id: request.session_id,
    payload: { session_id: request.session_id, ...payload },
  });
  return { api, args, sent, merged, pending, writes, frame };
}

const stored = (id) => ({ id, role: "user", content: `Message ${id}` });

test("history follows multiple pages to the server's last known row", async (t) => {
  const { api, args, sent, merged, writes, frame } = historyFixture(t, async () => ({
    messages: [stored("m0")], meta: { local_last_message_id: "m0", local_message_count: 1 },
  }));
  assert.equal(api.requestHistoryWithCache("session-a"), true);
  await tick();
  assert.equal(sent[0].type, "session_history_meta");
  assert.equal(api.handleSessionHistoryMessage(frame(sent[0], { can_delta: true, message_count: 102, last_message_id: "m101" })), true);
  const page = Array.from({ length: 100 }, (_, index) => stored(`m${index + 1}`));
  assert.equal(api.handleSessionHistoryMessage(frame(sent[1], { messages: page, count: 100 })), true);
  assert.equal(args.pendingHistorySessionIDRef.current, "session-a");
  assert.equal(sent[2].payload.after_message_id, "m100");
  assert.equal(api.handleSessionHistoryMessage(frame(sent[2], { messages: [stored("m101")], count: 1 })), true);
  await tick();
  assert.equal(args.historySessionIDRef.current, "session-a");
  assert.equal(args.pendingHistorySessionIDRef.current, "");
  assert.equal(merged.at(-1).messages.length, 102);
  assert.equal(writes[0].messages[101].id, "m101");
});

test("a missing history watermark triggers full sync and old connection replies never apply", async (t) => {
  const { api, args, sent, merged, frame } = historyFixture(t, async () => ({
    messages: [stored("m0")], meta: { local_last_message_id: "m0", local_message_count: 1 },
  }));
  api.requestHistoryWithCache("session-a");
  await tick();
  api.handleSessionHistoryMessage(frame(sent[0], { can_delta: true, message_count: 2, last_message_id: "m1" }));
  api.handleSessionHistoryMessage(frame(sent[1], { messages: [stored("other")], count: 1 }));
  assert.equal(sent[2].type, "session_history");
  const old = frame(sent[2], { messages: [stored("old")], count: 1 });
  args.relayScopeRef.current = { pairingID: "pairing-a", connectionID: "connection-b" };
  api.resetHistorySync();
  api.requestSessionHistoryFull("session-a");
  assert.equal(api.handleSessionHistoryMessage(old), false);
  assert.equal(api.handleSessionHistoryMessage(frame(sent[3], { messages: [stored("new")], count: 1 })), true);
  assert.deepEqual(merged.map((entry) => entry.messages.map((message) => message.id)), [["m0"], ["new"]]);
  await tick();
});

test("asset list accepts only the latest request for the selected session", (t) => {
  const { api, args, sent, frame } = historyFixture(t, undefined, useRemoteRequests);
  assert.equal(api.requestAssets("session-a"), true);
  args.sessionIDRef.current = "session-b";
  assert.equal(api.requestAssets("session-b"), true);
  args.sessionIDRef.current = "session-a";
  assert.equal(api.requestAssets("session-a"), true);
  assert.equal(api.acceptAssetList(frame(sent[0], { assets: [] })), false);
  assert.equal(api.acceptAssetList(frame(sent[1], { assets: [] })), false);
  assert.equal(api.acceptAssetList(frame(sent[2], { assets: [] })), true);
  assert.equal(api.acceptAssetList(frame(sent[2], { assets: [] })), false);
});

const mcpServers = [
  { name: "files", state: "loaded", inventory_tool_count: 3, exposed_tool_count: 2 },
  { name: "notes", state: "disabled", inventory_tool_count: 0, exposed_tool_count: 0 },
  { name: "search", state: "unavailable", inventory_tool_count: 0, exposed_tool_count: 0 },
];
const mcpList = (request, servers = mcpServers) => ({
  type: "mcp_list_result", request_id: request.request_id, payload: { servers },
});
const mcpReload = (request, reloaded = true) => ({
  type: "mcp_reload_result", request_id: request.request_id, payload: { reloaded, message: "agent detail" },
});
const mcpError = (request, payload) => ({ type: "error", request_id: request.request_id, payload });

function mcpFixture(t, { send = () => true, useTransport = false } = {}) {
  const scope = { current: { pairingID: pairing.pairingID, connectionID: "connection-a" } };
  const sent = [], states = [], chatErrors = [];
  const sendFromRelay = useTransport ? useRelaySender({
    socketRef: { current: { pairing, connectionID: "connection-a", send } },
    currentPairingRef: { current: pairing }, activeRequestIDRef: { current: "turn-a" }, sessionIDRef: { current: "session-a" },
    onEnqueueFailure: (message) => api.handleMessage(message), addErrorMessage: (message) => chatErrors.push(message),
  }) : send;
  const api = createMCPRequests({
    getScope: () => scope.current,
    sendEnvelope: (type, frame) => { sent.push({ type, ...frame }); return sendFromRelay(type, frame); },
    onChange: (state) => states.push(state),
  });
  t.after(api.clear);
  return { api, scope, sent, states, chatErrors, get state() { return states.at(-1); } };
}

test("MCP inventory and reload own separate global requests and accept explicit empty inventory", (t) => {
  const h = mcpFixture(t);
  assert.equal(h.api.refresh(), true);
  assert.equal(h.api.reload(), true);
  const [list, reload] = h.sent;
  assert.deepEqual(h.sent.map(({ request_id, ...frame }) => frame), [
    { type: "mcp_list", session_id: "", payload: {} },
    { type: "mcp_reload", session_id: "", payload: {} },
  ]);
  assert.notEqual(list.request_id, reload.request_id);
  assert.equal(h.api.refresh(), false);
  assert.equal(h.api.reload(), false);
  h.api.handleMessage(mcpReload(list));
  h.api.handleMessage(mcpList(reload));
  h.api.handleMessage(mcpError(list, { request_type: "mcp_reload" }));
  assert.equal(h.state.reloading, true);
  assert.deepEqual(h.state.inventory, { servers: null, status: "loading" });

  h.api.handleMessage(mcpList(list, []));
  assert.deepEqual(h.state.inventory, { servers: [], status: "ready" });
  assert.equal(h.state.reloading, true);
  h.api.handleMessage(mcpReload(reload, false));
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载失败/);
  assert.equal(h.sent.length, 2);
});

test("only a confirmed MCP reload replaces a pre-reload inventory request", (t) => {
  const h = mcpFixture(t);
  h.api.refresh();
  h.api.reload();
  const [before, reload] = h.sent;
  h.api.handleMessage(mcpReload(reload));
  const after = h.sent[2];
  assert.equal(after.type, "mcp_list");
  assert.notEqual(after.request_id, before.request_id);
  assert.match(h.state.reloadMessage, /重载成功/);
  h.api.handleMessage(mcpList(before));
  h.api.handleMessage(mcpError(before, { message: "unsupported client message type: mcp_list" }));
  assert.deepEqual(h.state.inventory, { servers: null, status: "loading" });
  h.api.handleMessage(mcpList(after));
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "ready" });
  h.api.handleMessage(mcpReload(reload));
  assert.equal(h.sent.length, 3);
});

test("same-pairing reconnect keeps stale MCP inventory and re-pair discards it and reload feedback", (t) => {
  const h = mcpFixture(t);
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent[0]));
  h.api.refresh();
  h.api.reload();
  const [, oldList, oldReload] = h.sent;
  h.scope.current = { ...h.scope.current, connectionID: "" };
  h.api.handleDisconnect();
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "idle" });
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载结果未确认/);
  assert.equal(h.api.refresh(), false);
  h.scope.current = { ...h.scope.current, connectionID: "connection-b" };
  h.api.refresh();
  h.api.handleMessage(mcpList(oldList, []));
  h.api.handleMessage(mcpReload(oldReload));
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "loading" });
  assert.equal(h.sent.length, 4);

  h.scope.current = { pairingID: "pairing-b", connectionID: "connection-b" };
  h.api.handleMessage(mcpList(h.sent[3], []));
  assert.deepEqual(h.state.inventory, { servers: null, status: "idle" });
  assert.equal(h.state.reloadMessage, "");
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent[4], []));
  assert.deepEqual(h.state.inventory, { servers: [], status: "ready" });
});

test("legacy MCP rejection, malformed inventory, and silent timeout keep the previous snapshot", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const h = mcpFixture(t);
  h.api.refresh();
  h.api.handleMessage(mcpList(h.sent[0]));
  for (const requestType of ["mcp_list", undefined]) {
    h.api.refresh();
    h.api.handleMessage(mcpError(h.sent.at(-1), { request_type: requestType, message: "unsupported client message type: mcp_list" }));
    assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "unsupported" });
  }
  h.api.refresh();
  h.api.handleMessage({ ...mcpList(h.sent.at(-1)), payload: {} });
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "error" });
  h.api.refresh();
  h.api.reload();
  t.mock.timers.tick(15_000);
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "timeout" });
  assert.equal(h.state.reloading, true);
  h.api.handleMessage(mcpList(h.sent.at(-2), []));
  assert.deepEqual(h.state.inventory, { servers: mcpServers, status: "timeout" });
  t.mock.timers.tick(45_000);
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载确认超时/);
  h.api.handleMessage(mcpReload(h.sent.at(-1)));
  assert.equal(h.sent.length, 6);
});

test("MCP transport failures stay in settings and cleanup ignores late results", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const frames = [];
  const h = mcpFixture(t, { useTransport: true, send: (raw) => {
    const frame = JSON.parse(raw);
    frames.push(frame);
    if (frame.type === "mcp_list") throw new Error("private transport detail");
    return Promise.resolve(false);
  } });
  assert.equal(h.api.refresh(), false);
  assert.equal(h.state.inventory.status, "error");
  assert.equal(h.api.reload(), true);
  await tick();
  assert.equal(h.state.reloading, false);
  assert.match(h.state.reloadMessage, /重载失败/);
  assert.deepEqual(h.chatErrors, []);
  assert.ok(frames.every((frame) => frame.session_id === "" && frame.request_id !== "turn-a"));

  const next = mcpFixture(t);
  next.api.refresh();
  next.api.reload();
  next.api.clear();
  const updates = next.states.length;
  t.mock.timers.tick(60_000);
  next.api.handleMessage(mcpList(next.sent[0]));
  next.api.handleMessage(mcpReload(next.sent[1]));
  assert.equal(next.states.length, updates);
  assert.equal(next.sent.length, 2);
});
