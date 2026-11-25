const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const { test } = require("node:test");
const ts = require("typescript");

for (const extension of [".ts", ".tsx"]) {
  require.extensions[extension] = (module, filename) => {
    const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
      fileName: filename,
    });
    module._compile(outputText, filename);
  };
}

const {
  createAssetImagePreviewLoader, decodeAssetImagePreview, maxPreviewBytes, pickedImageURI,
} = require("../src/utils/assetImagePreview.ts");
const { assistantReplyAssets, collectMarkdownAssetImages, markdownImage, parseMarkdownBlocks } = require("../src/utils/chatMarkdown.ts");
const { historyMessageToChatItem } = require("../src/utils/chatHistory.ts");
const { buildChatTurns } = require("../src/utils/chatRenderItems.ts");
const { messageWithAttachedFiles } = require("../src/utils/attachments.ts");
const { assetSummariesForSession, mergeAssetSummaries } = require("../src/utils/toolAssets.ts");
const { startRelayWebSocket } = require("../src/utils/relayWebSocket.ts");

const noop = () => {};
const jsx = (type, props, key) => ({ type, props: props || {}, key });
const openLinks = [];
let activeRender;
const react = {
  createContext: (defaultValue) => ({ defaultValue }),
  useCallback: (fn) => fn,
  useMemo: (fn) => fn(),
  useContext: (context) => activeRender.contexts.get(context) ?? context.defaultValue,
  useRef: (initial) => activeRender.slot(() => ({ current: initial })),
  useState(initial) {
    const state = activeRender.slot(() => ({ value: typeof initial === "function" ? initial() : initial }));
    return [state.value, (update) => { state.value = typeof update === "function" ? update(state.value) : update; }];
  },
  useEffect(setup, deps) {
    const render = activeRender;
    const slot = render.slot(() => ({}));
    if (!slot.deps || deps.some((dep, index) => !Object.is(dep, slot.deps[index]))) {
      render.effects.push(() => {
        slot.cleanup?.();
        slot.deps = deps;
        slot.cleanup = setup();
      });
    }
  },
};

const originalLoad = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") return react;
  if (name === "react/jsx-runtime") return { Fragment: "Fragment", jsx, jsxs: jsx };
  if (name === "expo-image") return { Image: "Image" };
  if (name === "@expo/vector-icons") return { Feather: "Feather" };
  if (name === "expo-haptics") return {};
  if (name === "react-native-svg") return { SvgXml: "SvgXml" };
  if (name === "react-native") return {
    ActivityIndicator: "ActivityIndicator", Modal: "Modal", Pressable: "Pressable", ScrollView: "ScrollView", Text: "Text", View: "View",
    Linking: { openURL: async (url) => { openLinks.push(url); } }, Platform: { OS: "web" },
    StyleSheet: { create: (styles) => styles, flatten: (style) => Object.assign({}, ...[style].flat()) },
  };
  if (name === "../../theme/ThemeProvider") return { useAppTheme: () => ({ palette: {} }) };
  if (name === "./styles") return { useChatStyles: () => new Proxy({}, { get: (_, key) => key }) };
  if (name === "../../utils/clipboard") return {};
  return originalLoad.call(this, name, ...args);
};
let useChatMessages, useRemoteMessageHandler, useRelaySender, AssetPreviewConnectionContext, AssetPreviewVisibleContext;
let ChatAttachmentCard, SharedAssetCard, MarkdownText, AssistantReply;
try {
  ({ useChatMessages } = require("../src/hooks/useChatMessages.ts"));
  ({ useRemoteMessageHandler } = require("../src/hooks/useRemoteMessageHandler.ts"));
  ({ useRelaySender } = require("../src/hooks/useRelaySender.ts"));
  ({ AssetPreviewConnectionContext, AssetPreviewVisibleContext } = require("../src/hooks/useAssetImagePreview.ts"));
  ({ ChatAttachmentCard } = require("../src/components/chat/ChatAttachmentCard.tsx"));
  ({ SharedAssetCard } = require("../src/components/chat/SharedAssetCard.tsx"));
  ({ MarkdownText } = require("../src/components/chat/MarkdownText.tsx"));
  ({ AssistantReply } = require("../src/components/chat/AssistantReply.tsx"));
} finally {
  Module._load = originalLoad;
}

function mount(t, component, contexts = new Map()) {
  const slots = [];
  t.after(() => slots.forEach((slot) => slot.cleanup?.()));
  return (props) => {
    let index = 0;
    const effects = [];
    activeRender = {
      contexts, effects,
      slot(init) {
        if (slots.length === index) slots.push(init());
        return slots[index++];
      },
    };
    try { return component(props); }
    finally {
      activeRender = undefined;
      effects.forEach((setup) => setup());
    }
  };
}

function nodes(element, predicate) {
  if (Array.isArray(element)) return element.flatMap((child) => nodes(child, predicate));
  if (!element || typeof element !== "object") return [];
  return [...(predicate(element) ? [element] : []), ...nodes(element.props?.children, predicate)];
}

const jpegPayload = (assetID, fields = {}) => ({ asset_id: assetID, content_type: "image/jpeg", data: "/9j/2Q==", width: 80, height: 60, ...fields });
const previewResult = (request, fields = {}, payload = {}) => ({
  type: "asset_image_preview_result", request_id: request.request_id, session_id: request.session_id,
  payload: jpegPayload(request.payload.asset_id, payload), ...fields,
});
const previewURI = "data:image/jpeg;base64,/9j/2Q==";
const buttonFeedback = (style) => style;

function previewHarness(t, options = {}) {
  let scope = { pairingID: "pair", connectionID: "c1" };
  let session = "s1";
  const requests = [];
  const loader = createAssetImagePreviewLoader({
    getScope: () => scope, isSessionCurrent: (id) => id === session,
    sendEnvelope: (type, req) => { requests.push({ type, ...req }); return true; }, ...options,
  });
  t.after(loader.clear);
  const subscribe = (assetID, sessionID = session) => {
    const results = [];
    const stop = loader.subscribe(sessionID, assetID, (result) => results.push(result));
    return { results, stop };
  };
  return {
    loader, requests, subscribe,
    switchSession(next) { session = next; loader.prune(); },
    switchConnection(next, pairingID = scope.pairingID) { scope = { pairingID, connectionID: next }; loader.prune(); },
    accept(request, fields, payload) { return loader.handleMessage(previewResult(request, fields, payload)); },
  };
}

test("preview requests deduplicate and accept only the exact request, session and asset", (t) => {
  const h = previewHarness(t);
  const first = h.subscribe("ast_a");
  const duplicate = h.subscribe("ast_a");
  assert.equal(h.requests.length, 1);
  const request = h.requests[0];
  assert.equal(request.type, "asset_image_preview");
  assert.equal(request.session_id, "s1");
  assert.match(request.request_id, /^asset-image-preview-/);
  assert.deepEqual(request.payload, { asset_id: "ast_a" });
  h.accept(request, { request_id: "other" });
  h.accept(request, { session_id: undefined });
  h.accept(request, { session_id: "s2" });
  h.accept(request, {}, { asset_id: "ast_b" });
  assert.deepEqual(first.results, []);
  h.accept(request);
  const result = { status: "ready", image: { uri: previewURI, width: 80, height: 60 } };
  assert.deepEqual(first.results, [result]);
  assert.deepEqual(duplicate.results, [result]);
  assert.deepEqual(h.subscribe("ast_a").results, [result]);
  assert.equal(h.requests.length, 1);
});

test("concurrency and queue are bounded and an offscreen request leaves the queue", (t) => {
  const h = previewHarness(t, { concurrency: 2 });
  h.subscribe("ast_a"); h.subscribe("ast_b");
  const canceled = h.subscribe("ast_c");
  h.subscribe("ast_d");
  for (let i = 0; i < 30; i++) h.subscribe(`ast_more_${i}`);
  assert.equal(h.requests.length, 2);
  assert.deepEqual(h.subscribe("ast_over_limit").results, [{ status: "error", reason: "busy" }]);
  canceled.stop();
  h.accept(h.requests[0]);
  assert.equal(h.requests[2].payload.asset_id, "ast_d");
  assert.deepEqual(canceled.results, []);
});

test("cache expires, evicts least-recent entries, and discards a failed decoder result", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"] });
  const h = previewHarness(t, { cacheLimit: 2, cacheTTL: 100 });
  for (const id of ["ast_a", "ast_b"]) {
    h.subscribe(id);
    h.accept(h.requests.at(-1));
  }
  const cachedA = h.subscribe("ast_a").results[0].image;
  h.subscribe("ast_c");
  h.accept(h.requests.at(-1));
  assert.equal(h.subscribe("ast_a").results[0].image, cachedA);
  assert.equal(h.subscribe("ast_b").results.length, 0);
  h.accept(h.requests.at(-1));
  h.loader.discard("s1", "ast_a", cachedA);
  assert.equal(h.subscribe("ast_a").results.length, 0);
  h.accept(h.requests.at(-1));
  t.mock.timers.tick(101);
  assert.equal(h.subscribe("ast_b").results.length, 0);
  assert.equal(h.requests.length, 6);
});

test("session changes, disconnect and re-pair retire old requests and clear preview cache", (t) => {
  const h = previewHarness(t);
  h.subscribe("ast_cache");
  h.accept(h.requests[0]);
  const left = h.subscribe("ast_a");
  const old = h.requests.at(-1);
  h.switchSession("s2");
  const current = h.subscribe("ast_a");
  const currentRequest = h.requests.at(-1);
  assert.equal(currentRequest.session_id, "s2");
  h.accept(old);
  assert.deepEqual(left.results, [{ status: "error", reason: "offline" }]);
  assert.deepEqual(current.results, []);
  h.switchConnection("");
  assert.deepEqual(current.results, [{ status: "error", reason: "offline" }]);
  assert.deepEqual(h.subscribe("ast_a").results, [{ status: "error", reason: "offline" }]);
  h.switchConnection("c2");
  const retried = h.subscribe("ast_a");
  h.accept(currentRequest);
  assert.deepEqual(retried.results, []);
  h.accept(h.requests.at(-1));
  assert.equal(retried.results[0].image.uri, previewURI);
  h.switchSession("s1");
  assert.deepEqual(h.subscribe("ast_cache").results, []);
  h.accept(h.requests.at(-1));
  h.switchConnection("c2", "second-pair");
  assert.deepEqual(h.subscribe("ast_cache").results, []);
});

test("only bounded typed JPEG/PNG/GIF/WebP payloads are usable and old Relay errors stay on the preview", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = previewHarness(t, { requestTimeoutMs: 50 });
  const legacy = h.subscribe("ast_legacy");
  const failingRequest = h.requests.at(-1);
  assert.equal(h.loader.handleMessage({ type: "error", request_id: failingRequest.request_id,
    session_id: "s1", payload: { message: "unsupported client message type: asset_image_preview" } }), true);
  assert.deepEqual(legacy.results, [{ status: "error", reason: "unsupported" }]);
  const malformed = h.subscribe("ast_malformed");
  h.accept(h.requests.at(-1), {}, { content_type: "image/svg+xml" });
  assert.deepEqual(malformed.results, [{ status: "error", reason: "invalid" }]);
  for (const fields of [{ width: 2049 }, { height: 0 }, { width: 2.5 }, { width: "80" }, { data: "data:image/jpeg;base64,/9j/" }, { data: "/9j/2Q=A" }]) {
    assert.equal(decodeAssetImagePreview(jpegPayload("ast_bad", fields)), null);
  }
  for (const size of [maxPreviewBytes, maxPreviewBytes + 1]) {
    const bytes = Buffer.alloc(size);
    bytes.set([255, 216, 255]);
    assert.equal(Boolean(decodeAssetImagePreview(jpegPayload("ast_image", { data: bytes.toString("base64") }))), size === maxPreviewBytes);
  }
  const formats = [
    ["image/jpeg", Buffer.from([0xff,0xd8,0xff,0xd9])],
    ["image/png", Buffer.from([137,80,78,71,13,10,26,10])],
    ["image/gif", Buffer.from("GIF89a")],
    ["image/webp", Buffer.from("RIFF\x04\x00\x00\x00WEBP")],
  ];
  for (const [content_type, bytes] of formats) {
    const preview=decodeAssetImagePreview({asset_id:"ast_format",content_type,data:bytes.toString("base64"),width:32,height:24});
    assert.equal(preview.uri,`data:${content_type};base64,${bytes.toString("base64")}`);
  }
  assert.equal(decodeAssetImagePreview({asset_id:"ast_bad",content_type:"image/webp",data:Buffer.from("RIFFxxxxWAVE").toString("base64"),width:10,height:10}),null);
  const unanswered = h.subscribe("ast_absent");
  t.mock.timers.tick(50);
  assert.deepEqual(unanswered.results, [{ status: "error", reason: "timeout" }]);
  assert.equal(h.loader.handleMessage({ type: "error", request_id: "ordinary", payload: { request_type: "user_message" } }), false);
  assert.deepEqual(h.subscribe("https://example.invalid/image").results, [{ status: "error", reason: "invalid" }]);
  assert.equal(h.requests.length, 3);
});

test("Agent preview errors identify the failing stage on the matching asset", (t) => {
  const h = previewHarness(t);
  for (const [code, reason] of [
    ["asset_preview_attachment_unavailable", "attachment"],
    ["asset_preview_download_failed", "download"],
    ["asset_preview_invalid_image", "invalid"],
    ["asset_preview_too_large", "too_large"],
    ["asset_preview_timeout", "timeout"],
  ]) {
    const subscription = h.subscribe(`ast_${reason}`);
    const request = h.requests.at(-1);
    const message = {
      type: "error", request_id: request.request_id, session_id: request.session_id,
      payload: { request_type: "asset_image_preview", asset_id: request.payload.asset_id, code },
    };
    h.loader.handleMessage({ ...message, session_id: "another-session" });
    assert.deepEqual(subscription.results, []);
    h.loader.handleMessage(message);
    assert.deepEqual(subscription.results, [{ status: "error", reason }]);
  }
});

test("an authenticated connection sends the explicit session without touching the chat request", (t) => {
  const originalWebSocket = globalThis.WebSocket;
  const sockets = [];
  globalThis.WebSocket = class FakeSocket {
    static OPEN = 1;
    constructor() { this.sent = []; sockets.push(this); }
    send(raw) { this.sent.push(JSON.parse(raw)); }
    close() { this.readyState = 3; }
    open() { this.readyState = 1; this.onopen(); }
    receive(message) { this.onmessage?.({ data: JSON.stringify(message) }); }
  };
  t.after(() => { globalThis.WebSocket = originalWebSocket; });
  const pairing = { pairingID: "pair", relayURL: "https://relay.example.invalid", userID: "owner", deviceID: "agent", clientToken: "test-token" };
  const scope = { pairingID: pairing.pairingID, connectionID: "" };
  const activeRequestIDRef = { current: "in-progress-chat" };
  const socketRef = { current: null };
  const errors = [];
  const send = useRelaySender({ activeRequestIDRef, socketRef, currentPairingRef: { current: pairing },
    sessionIDRef: { current: "parent" }, addErrorMessage: (error) => errors.push(error), onEnqueueFailure: (message) => loader.handleMessage(message) });
  const loader = createAssetImagePreviewLoader({ getScope: () => scope, isSessionCurrent: (session) => session === "child", sendEnvelope: send });
  t.after(loader.clear);
  function connect() {
    const close = startRelayWebSocket(pairing, {
      onAuthenticating: noop,
      onAuthenticated: (handle) => { socketRef.current = handle; scope.connectionID = handle.connectionID; loader.prune(); },
      onLost: () => { socketRef.current = null; scope.connectionID = ""; loader.prune(); },
      onMessage: loader.handleMessage,
    });
    t.after(close);
    return sockets.at(-1);
  }
  const identity = { user_id: pairing.userID, device_id: pairing.deviceID };
  const auth = (socket) => socket.receive({ ...identity, type: "heartbeat", request_id: socket.sent[0].request_id, payload: { received: "heartbeat" } });
  const a = connect();
  a.open();
  loader.subscribe("child", "ast_image", noop);
  assert.equal(a.sent.length, 1);
  auth(a);
  const first = [];
  loader.subscribe("child", "ast_image", (result) => first.push(result));
  const request = a.sent[1];
  assert.deepEqual(request, { ...identity, client_token: pairing.clientToken, type: "asset_image_preview",
    session_id: "child", request_id: request.request_id, payload: { asset_id: "ast_image" } });
  assert.equal(activeRequestIDRef.current, "in-progress-chat");
  a.receive(previewResult(request, { ...identity, device_id: "another-device" }));
  assert.deepEqual(first, []);
  const late = a.onmessage;
  a.onclose();
  const b = connect();
  b.open(); auth(b);
  const second = [];
  loader.subscribe("child", "ast_image", (result) => second.push(result));
  late({ data: JSON.stringify(previewResult(request, identity)) });
  b.receive(previewResult(request, identity));
  assert.deepEqual(second, []);
  b.receive(previewResult(b.sent[1], identity));
  assert.deepEqual(first, [{ status: "error", reason: "offline" }]);
  assert.equal(second[0].image.uri, previewURI);
  assert.deepEqual(errors, []);
});

test("asset-only foreground and background completions survive history sync and show each trusted output once", (t) => {
  const state = mount(t, useChatMessages)();
  const sessionIDRef = { current: "s1" };
  const remote = mount(t, useRemoteMessageHandler)(new Proxy({
    ...state, sessionIDRef, requestSessionMapRef: { current: {} }, activeRequestIDRef: { current: "" },
  }, { get: (target, key) => target[key] ?? noop }));
  const png = { asset_id: "ast_png", file_name: "photo.png", content_type: "image/png", size: 51 };
  const summary = { ...png, id: "db-1", session_id: "s1", request_id: "req", tool_call_id: "tool-1", tool_name: "share_file" };
  const read = { ...summary, id: "db-2", asset_id: "ast_read", tool_call_id: "tool-2", tool_name: "get_asset_info" };
  const foreign = { ...summary, id: "db-3", asset_id: "ast_foreign", session_id: "s2" };
  const shareURL = "https://files.example.invalid/once";
  state.addMessage("s1", "user", "Please share", "req");
  state.addToolResult("s1", "share_file", "", JSON.stringify({ ...png, short_url: shareURL }), false, "req", undefined, "tool-1");
  state.addToolResult("s1", "get_asset_info", "", JSON.stringify({ ...png, asset_id: read.asset_id }), false, "req", undefined, "tool-2");
  remote({ type: "assistant_done", session_id: "s1", request_id: "req", payload: { message_id: "reply", assets: [{ ...summary, data: "must not persist" }, read, foreign] } });
  const catalog = assetSummariesForSession([summary, read, foreign], "s1");
  const chat = state.getSessionChat("s1");
  const reply = chat.messages.find((row) => row.finalReply);
  assert.deepEqual(reply.assets.map((asset) => asset.asset_id), ["ast_png", "ast_read"]);
  assert.equal(reply.assets[0].data, undefined);
  const stored = historyMessageToChatItem({ id: "reply", role: "assistant", content: "![caption](asset:ast_png)\n![again](asset:ast_png)" });
  state.mergeSessionHistory("s1", [stored], "server");
  const turn = buildChatTurns(state.getSessionChat("s1").messages, [], catalog).find((item) => item.type === "agent_turn");
  const cards = assistantReplyAssets(turn.text, turn.assets, catalog);
  assert.deepEqual(cards.map(({ asset, imageReference }) => [asset.assetID, asset.fileName, asset.shortURL, imageReference]), [
    ["ast_png", "caption", shareURL, true],
  ]);
  assert.deepEqual(assistantReplyAssets("![read](asset:ast_read)", [], catalog).map(({ asset }) => asset.assetID), ["ast_read"]);

  const background = { ...summary, id: "db-b", asset_id: "ast_background", session_id: "s2", request_id: "turn-b", tool_name: "attach_image" };
  remote({ type: "background_turn_event", request_id: "turn-b", session_id: "s2", payload: {
    kind: "completed", sequence: 2, session_id: "s2", turn_id: "turn-b", assets: [background],
  } });
  assert.deepEqual(buildChatTurns(state.getSessionChat("s2").messages)[0].assets.map((asset) => asset.assetID), ["ast_background"]);
  assert.equal(state.getSessionChat("s1").messages.some((row) => row.requestID === "turn-b"), false);
});

test("reopened history backfills by exact tool call and aliases subagent record IDs", () => {
  const stored = (row) => historyMessageToChatItem(row);
  const assets = [{ id: "row-a", asset_id: "ast_a", tool_name: "attach_image", tool_call_id: "a", file_name: "upload.png" },
    { id: "row-b", asset_id: "ast_b", tool_name: "share_file", tool_call_id: "b", file_name: "late.jpg" },
    { id: "row-c", asset_id: "ast_c", tool_name: "get_asset_info", tool_call_id: "a", file_name: "read.jpg" }];
  const history = [
    stored({ id: "u1", role: "user", content: "One" }),
    stored({ id: "a1", role: "tool", tool_name: "attach_image", tool_call_id: "a", tool_truncated: true, content: "partial" }),
    stored({ id: "r1", role: "assistant", content: "done" }),
    stored({ id: "u2", role: "user", content: "Two" }),
    stored({ id: "a2", role: "tool", tool_name: "share_file", tool_call_id: "b", tool_status: "failed", content: "error" }),
    stored({ id: "r2", role: "assistant", content: "couldn't share" }),
  ];
  const turns = buildChatTurns(history, [], assets).filter((item) => item.type === "agent_turn");
  assert.deepEqual(turns.map((turn) => turn.assets?.map((asset) => asset.assetID) || []), [["ast_a"], []]);
  const legacyChild = { id: "db-child", short_url: "https://files.example.invalid/shared" };
  const liveChild = { id: "db-child", asset_id: "ast_child", file_name: "child.png" };
  assert.deepEqual(mergeAssetSummaries([liveChild], [legacyChild]), [{ ...liveChild, short_url: legacyChild.short_url }]);
});

test("local JPEG and PNG win before private fallback, history rehydrates metadata, and both open full screen", (t) => {
  const h = previewHarness(t);
  const contexts = new Map([[AssetPreviewConnectionContext, { connectionID: "c1", loader: h.loader }]]);
  const render = (node, name = node.type) => mount(t, name, contexts)(node.props);
  for (const [assetID, extension, contentType] of [["ast_jpg", "jpg", "image/jpeg"], ["ast_png", "png", "image/png"]]) {
    const initialRequestCount = h.requests.length;
    const file = { kind: "uploaded_asset", asset_id: assetID, file_name: `picked.${extension}`, content_type: contentType,
      size: 100, local_uri: `file:///cache/picked.${extension}`, short_url: "https://files.example.invalid/once" };
    const renderCard = mount(t, SharedAssetCard, contexts);
    const initial = render({ type: ChatAttachmentCard, props: { attachment: file, buttonFeedback, sessionID: "s1" } });
    assert.equal(initial.type, SharedAssetCard);
    const findPreview = (card) => nodes(card, (node) => typeof node.type === "function" && node.props.assetID === assetID)[0];
    const local = findPreview(renderCard(initial.props));
    const renderSource = mount(t, local.type, contexts);
    const source = renderSource(local.props);
    const renderImage = mount(t, source.type, contexts);
    let imageTree = renderImage(source.props);
    const thumbnail = nodes(imageTree, (node) => node.type === "Image")[0];
    assert.equal(thumbnail.props.source.uri, file.local_uri);
    thumbnail.props.onLoad();
    imageTree = renderImage(source.props);
    nodes(imageTree, (node) => node.type === "Pressable" && node.props.accessibilityRole === "button")[0].props.onPress();
    const modal = nodes(renderImage(source.props), (node) => node.type === "Modal")[0];
    assert.equal(nodes(modal, (node) => node.type === "Image")[0].props.source.uri, file.local_uri);
    assert.equal(h.requests.length, initialRequestCount);
    thumbnail.props.onError();
    const fallback = renderSource(local.props);
    const renderRemote = mount(t, fallback.type, contexts);
    renderRemote(fallback.props);
    h.accept(h.requests.at(-1));
    const relayImage = renderRemote(fallback.props);
    assert.equal(relayImage.props.uri, previewURI);
    relayImage.props.onError();
    assert.equal(renderRemote(fallback.props).props.reason, "invalid");

    const newURI = `file:///cache/new.${extension}`;
    const changed = findPreview(renderCard({ ...initial.props, localURI: newURI }));
    assert.notEqual(changed.key, local.key);
    assert.equal(render(changed).props.uri, newURI);

    const modelText = messageWithAttachedFiles("show", [file]);
    assert.doesNotMatch(modelText, /file:\/\/|https:\/\/|base64/);
    const reopened = historyMessageToChatItem({ id: "stored", role: "user", content: modelText });
    assert.equal(reopened.attachments[0].local_uri, undefined);
    const rehydrated = render({ type: ChatAttachmentCard, props: { attachment: reopened.attachments[0], buttonFeedback, sessionID: "s1" } });
    const historyPreview = findPreview(renderCard(rehydrated.props));
    const historyRemote = render(historyPreview);
    const renderHistoryRemote = mount(t, historyRemote.type, contexts);
    renderHistoryRemote(historyRemote.props);
    h.accept(h.requests.at(-1));
    const historyImage = renderHistoryRemote(historyRemote.props);
    const renderHistoryImage = mount(t, historyImage.type, contexts);
    nodes(renderHistoryImage(historyImage.props), (node) => node.type === "Image")[0].props.onLoad();
    nodes(renderHistoryImage(historyImage.props), (node) => node.props.accessibilityRole === "button")[0].props.onPress();
    const historyModal = nodes(renderHistoryImage(historyImage.props), (node) => node.type === "Modal")[0];
    assert.equal(nodes(historyModal, (node) => node.type === "Image")[0].props.source.uri, previewURI);
  }
});

test("markdown renders external image captions as click-only links and never promotes incidental strings", async (t) => {
  openLinks.length = 0;
  const externalURL = "https://files.example.invalid/once";
  const text = [
    `![Screenshot](${externalURL} "title")`,
    "![data](data:image/jpeg;base64,/9j/) ![bad](file:///tmp/a.png)",
    "asset:ast_bare [label](asset:ast_link)",
    "\\![escaped](asset:ast_escape) `![inline code](asset:ast_inline)`",
    "~~~example\n![fenced](asset:ast_fenced)\n~~~",
    "    ![indented](asset:ast_indented)",
    "> ```\n> ![quoted code](asset:ast_quoted_code)\n> ```",
    "![accepted](asset:ast_ok) **![duplicated](asset:ast_ok)**",
  ].join("\n\n");
  assert.deepEqual([...collectMarkdownAssetImages(parseMarkdownBlocks(text))], [["ast_ok", "accepted"]]);
  assert.deepEqual(markdownImage(`![](<${externalURL}>)`), { kind: "external", url: externalURL, alt: "" });
  for (const value of [externalURL, "data:image/jpeg;base64,/9j/", "file://server/share/remote.jpg"]) {
    assert.equal(pickedImageURI(value), undefined);
  }
  const rendered = mount(t, MarkdownText)({ text });
  const links = nodes(rendered, (node) => node.props.url === externalURL);
  assert.equal(links.length, 1);
  assert.equal(links[0].props.children, "Screenshot");
  assert.deepEqual(openLinks, []);
  await mount(t, links[0].type)(links[0].props).props.onPress();
  assert.deepEqual(openLinks, [externalURL]);
  const reply = mount(t, AssistantReply)({ text, sessionID: "s1", buttonFeedback });
  assert.deepEqual(nodes(reply, (node) => node.type === SharedAssetCard).map((node) => node.props.asset.assetID), ["ast_ok"]);
  const offscreen = mount(t, SharedAssetCard, new Map([[AssetPreviewVisibleContext, false]]))({
    asset: { assetID: "ast_ok", contentType: "image/png", shortURL: externalURL }, sessionID: "s1", buttonFeedback,
  });
  assert.equal(nodes(offscreen, (node) => typeof node.type === "function").length, 0);
  assert.deepEqual(openLinks, [externalURL]);
});
