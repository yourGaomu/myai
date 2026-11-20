const assert = require("node:assert/strict");
const { test } = require("node:test");
const fs = require("node:fs");
const Module = require("node:module");
const ts = require("typescript");

require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  });
  module._compile(outputText, filename);
};

// These hooks only need state, refs and cleanup. Drive render boundaries and
// asynchronous picker/transport events without booting Expo or a browser.
let rendering;
let picker;
const react = {
  useCallback(fn) { return fn; },
  useRef(initial) { return rendering.slot(() => ({ current: initial })); },
  useState(initial) {
    const state = rendering.slot(() => ({ value: typeof initial === "function" ? initial() : initial }));
    return [state.value, (update) => { state.value = typeof update === "function" ? update(state.value) : update; }];
  },
  useEffect(setup) {
    const effects = rendering.effects;
    rendering.slot(() => { effects.push(setup); });
  },
};
const fileSystem = {
  FileMode: { ReadOnly: "read-only" },
  File: class {
    size = 1;
    open() { return { readBytes: () => Uint8Array.of(1), close() {} }; }
  },
};
const load = Module._load;
Module._load = function (name, ...args) {
  if (name === "react") return react;
  if (name === "expo-document-picker") return { getDocumentAsync: () => picker() };
  if (name === "expo-file-system") return fileSystem;
  if (name === "react-native") return { Platform: { OS: "android" } };
  return load.call(this, name, ...args);
};
let useFileState, useFileActions, useKnowledgeActions, useAssetUpload;
try {
  ({ useFileState } = require("../src/hooks/useFileState.ts"));
  ({ useFileActions } = require("../src/hooks/useFileActions.ts"));
  ({ useKnowledgeActions } = require("../src/hooks/useKnowledgeActions.ts"));
  ({ useAssetUpload } = require("../src/hooks/useAssetUpload.ts"));
} finally {
  Module._load = load;
}
const { assetUploadChunkSize, assetUploadMaxFileBytes } = require("../src/utils/assetUpload.ts");

function mount(fn, t) {
  const slots = [];
  const cleanups = [];
  t.after(() => { for (const cleanup of cleanups) cleanup?.(); });
  return (props) => {
    let index = 0;
    const effects = [];
    rendering = {
      effects,
      slot(init) {
        if (index === slots.length) slots.push(init());
        return slots[index++];
      },
    };
    try { return fn(props); }
    finally {
      rendering = undefined;
      for (const setup of effects) cleanups.push(setup());
    }
  };
}

function deferred() {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
}

const tick = () => new Promise((resolve) => setImmediate(resolve));
const asset = { uri: "file:///test/report.txt", name: "report.txt", mimeType: "text/plain", size: 1 };
const picked = { canceled: false, assets: [asset] };
const uploaded = { asset_id: "asset-1", file_name: asset.name, size: 1 };
const pairing = { pairingID: "pairing-1", connectionID: "connection-1" };
const base = (id) => ({ id, name: `Base ${id}`, rag_enabled: true });

function fileHarness(t, uploadMobileAsset) {
  const relayScopeRef = { current: { ...pairing } };
  const sessionIDRef = { current: "one" };
  const errors = [];
  const navigation = [];
  const pending = [];
  const sent = [];
  let accepting = true;
  const render = mount(({ pairingID, sessionID }) => {
    const state = useFileState({ pairingID, sessionID });
    return { ...state, ...useFileActions({
      ...state,
      addErrorMessage: (message) => errors.push([sessionID, message]),
      relayScopeRef, pairingID, sessionID, sessionIDRef, uploadMobileAsset,
      sendEnvelope: (type, envelope) => { sent.push({ type, ...envelope }); return accepting; },
      selectedChange: "",
      requestFiles: () => true,
      setViewMode: (mode) => navigation.push(mode),
      startPending: (operation) => pending.push([operation, true]),
      stopPending: (operation) => pending.push([operation, false]),
    }) };
  }, t);
  return {
    pending, errors, navigation, relayScopeRef, sessionIDRef, sent,
    render(sessionID = sessionIDRef.current, pairingID = relayScopeRef.current.pairingID) {
      sessionIDRef.current = sessionID;
      return render({ pairingID, sessionID });
    },
    setAccepting(value) { accepting = value; },
  };
}

test("a late upload completes only into its initiating chat draft and never navigates", async (t) => {
  const transfer = deferred();
  const targets = [];
  picker = async () => picked;
  const h = fileHarness(t, (options) => { targets.push(options); return transfer.promise; });
  let one = h.render("one");
  one.setMessageInput("Draft one");
  const inFlight = one.uploadLocalFile();
  await tick();
  assert.deepEqual(targets.map(({ sessionID, scope }) => ({ sessionID, scope })), [{ sessionID: "one", scope: pairing }]);
  const two = h.render("two");
  assert.equal(two.messageInput, "");
  two.setMessageInput("Draft two");
  transfer.resolve(uploaded);
  await inFlight;
  assert.deepEqual(h.navigation, []);
  assert.equal(h.render("two").messageInput, "Draft two");
  assert.deepEqual(h.render("two").attachedFiles, []);
  one = h.render("one");
  assert.equal(one.messageInput, "Draft one");
  assert.equal(one.attachedFiles[0].asset_id, "asset-1");

  h.setAccepting(false);
  assert.equal(one.sendMessageWithFiles(one.messageInput, "send-fail"), false);
  assert.equal(h.render("one").attachedFiles.length, 1);
  assert.equal(h.render("one").messageInput, "Draft one");
  h.setAccepting(true);
  assert.equal(one.sendMessageWithFiles(one.messageInput, "send-ok"), true);
  assert.equal(h.sent.at(-1).session_id, "one");
  assert.equal(h.render("one").messageInput, "");
  assert.deepEqual(h.render("one").attachedFiles, []);
  assert.equal(h.render("two").messageInput, "Draft two");
  assert.equal(h.render("two", "pairing-2").messageInput, "");
});

test("stale composer or picker cannot send under a new owner", async (t) => {
  const selection = deferred();
  picker = () => selection.promise;
  const h = fileHarness(t, async () => { assert.fail("stale picker uploaded"); });
  const one = h.render();
  const oldPick = one.uploadLocalFile();
  h.relayScopeRef.current = { pairingID: "pairing-2", connectionID: "connection-2" };
  assert.equal(one.sendMessageWithFiles("old text", "wrong-owner"), false);
  selection.resolve(picked);
  await oldPick;
  assert.deepEqual(h.sent, []);
  assert.deepEqual(h.errors, []);
  assert.deepEqual(h.render().attachedFiles, []);
});

test("disconnect releases the old picker and its finally cannot reset a replacement's pending UI", async (t) => {
  const first = deferred(), second = deferred();
  let calls = 0;
  picker = () => (calls++ === 0 ? first.promise : second.promise);
  const h = fileHarness(t, async () => uploaded);
  const screens = h.render();
  const stale = screens.uploadLocalFile();
  h.relayScopeRef.current = { ...pairing, connectionID: "" };
  screens.disconnectFileUpload();
  h.relayScopeRef.current = { ...pairing, connectionID: "connection-2" };
  const replacement = h.render("two").uploadLocalFile();
  first.resolve(picked);
  await stale;
  assert.deepEqual(h.pending, [["upload", true], ["upload", false], ["upload", true]]);
  assert.match(h.errors[0][1], /断开/);
  second.resolve(picked);
  await replacement;
  assert.equal(h.render("two").attachedFiles.length, 1);
  assert.deepEqual(h.render("one").attachedFiles, []);
});

function knowledgeHarness(t, transfer = async () => uploaded) {
  const relayScopeRef = { current: { ...pairing } };
  const sent = [], applied = [], errors = [], pending = [];
  const render = mount((args) => useKnowledgeActions({
    activeKnowledgeBase: base("one"), applyDocuments: (payload) => applied.push(payload),
    onError: (message) => errors.push(message), relayScopeRef, uploadMobileAsset: transfer,
    pairingID: pairing.pairingID, sessionID: "session-one", clientToken: "paired-test-fixture",
    sendEnvelope: (type, envelope) => { sent.push({ type, ...envelope }); return true; },
    startPending: (action) => pending.push([action, true]), stopPending: (action) => pending.push([action, false]),
    ...args,
  }), t);
  function result(request, knowledgeBaseID = request.payload.knowledge_base_id) {
    return {
      request_id: request.request_id, session_id: request.session_id,
      type: request.type === "knowledge_document_ingest" ? "knowledge_document_mutation_result" : "knowledge_document_list_result",
      payload: { knowledge_base_id: knowledgeBaseID, documents: [{ id: "doc-1", file_name: "report.txt" }], jobs: [] },
    };
  }
  return { relayScopeRef, sent, applied, errors, pending, render, result };
}

test("RAG captures the selected base and session, and only its exact reply can complete the import", async (t) => {
  const selection = deferred();
  const uploadTargets = [];
  picker = () => selection.promise;
  const h = knowledgeHarness(t, async (target) => { uploadTargets.push(target); return uploaded; });
  const inFlight = h.render().uploadDocument("one", "Base one");
  const other = { activeKnowledgeBase: base("two"), sessionID: "session-two" };
  h.render(other);
  selection.resolve(picked);
  await inFlight;
  assert.equal(uploadTargets[0].sessionID, "session-one");
  const [ingest] = h.sent;
  assert.equal(ingest.session_id, "session-one");
  assert.deepEqual(ingest.payload, { knowledge_base_id: "one", asset_id: "asset-1" });
  const actions = h.render(other);
  assert.equal(actions.uploadingDocument, false);
  assert.equal(actions.documentImportStatus.phase, "importing");
  assert.equal(actions.handleKnowledgeUploadMessage({ ...h.result(ingest), request_id: "unrelated" }), false);
  assert.equal(actions.handleKnowledgeUploadMessage(h.result(ingest, "two")), true);
  assert.equal(h.applied.length, 0);
  assert.equal(actions.handleKnowledgeUploadMessage(h.result(ingest)), true);
  assert.equal(h.render(other).documentImportStatus.phase, "complete");
  assert.equal(h.applied[0].knowledge_base_id, "one");
  assert.deepEqual(h.pending, []);
});

test("RAG disconnect lists the original base on reconnect, without re-ingest or name-based success", async (t) => {
  picker = async () => picked;
  const h = knowledgeHarness(t);
  await h.render().uploadDocument("one", "Base one");
  h.relayScopeRef.current = { ...pairing, connectionID: "" };
  h.render().disconnectKnowledgeUploads();
  assert.equal(h.render().documentImportStatus.phase, "unknown");
  h.relayScopeRef.current = { ...pairing, connectionID: "connection-2" };
  const other = { activeKnowledgeBase: base("two"), sessionID: "session-two" };
  h.render(other).reconnectKnowledgeUploads();
  const [ingest, listing] = h.sent;
  assert.equal(listing.type, "knowledge_document_list");
  assert.equal(listing.session_id, "session-one");
  assert.equal(listing.payload.knowledge_base_id, "one");
  const actions = h.render(other);
  assert.equal(actions.documentImportStatus.phase, "checking");
  actions.handleKnowledgeUploadMessage(h.result(listing));
  assert.equal(h.render(other).documentImportStatus.phase, "review");
  await h.render(other).uploadDocument("one", "Base one");
  assert.equal(h.sent.length, 2);
  // Unlike an equal filename, a late, correlated mutation reply proves success.
  actions.handleKnowledgeUploadMessage(h.result(ingest));
  assert.equal(h.render(other).documentImportStatus.phase, "complete");
});

test("RAG error and timeout leave an explicit review, rather than trying ingest twice", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  picker = async () => picked;
  const h = knowledgeHarness(t);
  await h.render().uploadDocument("one", "Base one");
  const [ingest] = h.sent;
  const actions = h.render();
  assert.equal(actions.handleKnowledgeUploadMessage({ type: "error", request_id: "otherwise-unrelated", payload: { request_type: "knowledge_document_ingest" } }), false);
  actions.handleKnowledgeUploadMessage({ type: "error", request_id: ingest.request_id, session_id: ingest.session_id,
    payload: { request_type: "knowledge_document_ingest", message: "indexing failed" } });
  assert.equal(h.sent[1].type, "knowledge_document_list");
  assert.match(h.render().documentImportStatus.message, /indexing failed/);
  t.mock.timers.tick(30_000);
  assert.equal(h.render().documentImportStatus.phase, "unknown");
  await h.render().uploadDocument("one", "Base one");
  assert.equal(h.sent.length, 2);
  h.render().dismissDocumentImport();
  assert.equal(h.render().documentImportStatus, null);
  await h.render().uploadDocument("two", "Base two");
  t.mock.timers.tick(330_000);
  assert.equal(h.sent.filter(({ type }) => type === "knowledge_document_ingest").length, 2);
  assert.equal(h.sent.at(-1).type, "knowledge_document_list");
  assert.equal(h.sent.at(-1).payload.knowledge_base_id, "two");
});

test("RAG never lists or applies a prior owner's import using the current pairing", async (t) => {
  picker = async () => picked;
  const h = knowledgeHarness(t);
  await h.render().uploadDocument("one", "Base one");
  const [ingest] = h.sent;
  h.relayScopeRef.current = { ...pairing, connectionID: "" };
  h.render().disconnectKnowledgeUploads();
  h.relayScopeRef.current = { pairingID: "pairing-2", connectionID: "connection-2" };
  const actions = h.render();
  actions.reconnectKnowledgeUploads();
  assert.equal(actions.handleKnowledgeUploadMessage(h.result(ingest)), true);
  assert.deepEqual(h.applied, []);
  assert.equal(h.sent.length, 1);
});

test("connection replacement after upload completion fences RAG before ingest", async (t) => {
  picker = async () => picked;
  const transfer = deferred();
  const h = knowledgeHarness(t, () => transfer.promise);
  const upload = h.render().uploadDocument("one", "Base one");
  await tick();
  transfer.resolve(uploaded);
  h.relayScopeRef.current = { ...pairing, connectionID: "connection-2" };
  await upload;
  assert.deepEqual(h.sent, []);
  assert.match(h.errors[0], /断开/);
  assert.equal(h.render().uploadingDocument, false);
});

test("null picker size falls through to the native file handle's size", async (t) => {
  const relayScopeRef = { current: { ...pairing } };
  const sent = [];
  const render = mount(() => useAssetUpload({
    relayScopeRef,
    sendEnvelope(type, envelope) {
      const operation = type.replace("asset_upload_", "");
      sent.push(operation);
      actions.handleAssetUploadMessage({
        type: "asset_upload_result", request_id: envelope.request_id, session_id: envelope.session_id,
        payload: { operation, upload_id: "upload-1", offset: operation === "begin" ? 0 : 1,
          chunk_size: assetUploadChunkSize, max_file_bytes: assetUploadMaxFileBytes,
          asset: operation === "complete" ? uploaded : undefined },
      });
      return true;
    },
  }), t);
  const actions = render();
  const result = await actions.uploadMobileAsset({ asset: { ...asset, size: null }, sessionID: "session-one", scope: pairing });
  assert.equal(result.asset_id, "asset-1");
  assert.deepEqual(sent, ["begin", "chunk", "complete"]);
});
