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

const noop = () => {};
const jsx = (type, props, key) => ({ type, props, key });
const platform = { OS: "android", select(choices) { return choices[this.OS] ?? choices.default; } };
const visibleContext = "AssetPreviewVisibleContext";
const keyboardListeners = new Set();
let activeRender;
const useEffect = (setup, deps) => {
  const render = activeRender;
  const slot = render.slot(() => ({}));
  if (!slot.deps || deps.some((dep, i) => !Object.is(dep, slot.deps[i]))) {
    render.effects.push(() => {
      slot.cleanup?.();
      slot.deps = deps;
      slot.cleanup = setup();
    });
  }
};
const react = {
  useCallback: (fn) => fn,
  useMemo: (fn) => fn(),
  useRef: (initial) => activeRender.slot(() => ({ current: initial })),
  useState(initial) {
    const slot = activeRender.slot(() => ({ value: initial }));
    return [slot.value, (update) => { slot.value = typeof update === "function" ? update(slot.value) : update; }];
  },
  useEffect,
  useLayoutEffect: useEffect,
};

const stubs = {
  react,
  "react/jsx-runtime": { Fragment: "Fragment", jsx, jsxs: jsx },
  "react-native": {
    ActivityIndicator: "ActivityIndicator", Pressable: "Pressable", ScrollView: "ScrollView", Text: "Text", View: "View",
    Platform: platform,
    StyleSheet: { create: (styles) => styles },
    Keyboard: {
      addListener(_name, listener) {
        keyboardListeners.add(listener);
        return { remove: () => keyboardListeners.delete(listener) };
      },
    },
    Animated: { View: "AnimatedView", Value: class { setValue() {} interpolate() {} } },
  },
  "@expo/vector-icons": { Feather: "Feather" },
  "../../hooks/useChatActions": { latestRegeneratableTurnID: noop },
  "../../hooks/useAssetImagePreview": { AssetPreviewVisibleContext: { Provider: visibleContext } },
  "../../theme/ThemeProvider": {
    useAppTheme: () => ({ palette: {} }),
    useThemedStyles: (create) => create({ font: { label: 12 }, palette: {}, shadow: {} }),
  },
  "../common/MotionEffects": { useReduceMotion: () => true },
};
for (const name of ["AgentRunTimeline", "AgentTurnCard", "AssistantLoadingBubble", "ChatEmptyState", "InlinePlanCard", "MessageBubble", "ToolActivityGroup"]) {
  stubs[`./${name}`] = { [name]: name };
}
stubs["../permissions/UserQuestionPrompt"] = { UserQuestionPrompt: "UserQuestionPrompt" };

const load = Module._load;
let ChatPanel, ChatJumpNav;
try {
  Module._load = function (name, ...args) {
    return stubs[name] ?? load.call(this, name, ...args);
  };
  ({ ChatPanel } = require("../src/components/chat/ChatPanel.tsx"));
  ({ ChatJumpNav } = require("../src/components/chat/ChatJumpNav.tsx"));
} finally {
  Module._load = load;
}
const { historyMessageToChatItem } = require("../src/utils/chatHistory.ts");

function renderComponent(component) {
  const slots = [];
  return {
    render(props) {
      let index = 0;
      const effects = [];
      activeRender = { effects, slot: (init) => (slots[index] ??= init(), slots[index++]) };
      try {
        return component(props);
      } finally {
        activeRender = undefined;
        effects.forEach((run) => run());
      }
    },
    unmount: () => slots.forEach((slot) => slot.cleanup?.()),
  };
}

function nodes(element, predicate) {
  if (Array.isArray(element)) return element.flatMap((child) => nodes(child, predicate));
  if (!element || typeof element !== "object") return [];
  return [...(predicate(element) ? [element] : []), ...nodes(element.props?.children, predicate)];
}
const find = (root, type) => nodes(root, (item) => item.type === type)[0];
const returnToBottom = (root) => nodes(root, (item) => item.props.accessibilityLabel === "回到底部")[0];
const layout = (y, height = 60) => ({ nativeEvent: { layout: { y, height } } });
const scroll = (y, height = 500, contentHeight = 1000) => ({
  nativeEvent: { contentOffset: { y }, layoutMeasurement: { height }, contentSize: { height: contentHeight } },
});
const history = () => [
  { id: "first", role: "user", content: "First   question" },
  { id: "assistant", role: "assistant", content: "Answer" },
  { id: "second", role: "user", content: "Second question" },
].map(historyMessageToChatItem);

function mountChat(t, os = "android") {
  platform.OS = os;
  const frames = [];
  const originalRAF = global.requestAnimationFrame;
  global.requestAnimationFrame = (fn) => frames.push(fn);
  let session, host, current;
  const sharedRef = { current: null };
  let props = {
    activeAssistantID: "", assets: [], buttonFeedback: (style) => style, chatScrollRef: sharedRef,
    connected: false, deviceID: "", greetingName: "", loadingHistory: true, messages: [],
    onPickSuggestion: noop, onRegenerate: noop, pendingRequestID: "", runs: [], sessionID: "s1", topClearance: 100,
  };
  t.after(() => {
    current.props.ref(null);
    session.component.unmount();
    global.requestAnimationFrame = originalRAF;
  });
  function render(nextProps = {}) {
    props = { ...props, ...nextProps };
    const panel = ChatPanel(props);
    if (!session || session.key !== panel.key) {
      current?.props.ref(null);
      session?.component.unmount();
      session = { key: panel.key, component: renderComponent(panel.type) };
      const calls = [];
      host = {
        calls, content: {}, getInnerViewNode() { return this.content; },
        scrollTo: (options) => calls.push(["jump", options]),
        scrollToEnd: (options) => calls.push(["tail", options]),
      };
    }
    const tree = session.component.render(panel.props);
    current = find(tree, "ScrollView");
    current.props.ref(host);
    return tree;
  }
  function row(tree, id, rowHost = {}) {
    const row = nodes(tree, (item) => item.props.rowID === id)[0];
    const component = renderComponent(row.type);
    const renderRow = (element = row) => {
      const node = component.render(element.props);
      if (node.props.ref) node.props.ref.current = rowHost;
      return node;
    };
    return { render: renderRow, component };
  }
  return {
    render, row, sharedRef,
    get calls() { return host.calls; },
    get content() { return host.content; },
    get scrollView() { return current.props; },
    flushFrames: () => frames.splice(0).forEach((cb) => cb()),
  };
}

test("synced history exposes rendered user IDs and uses the latest row offset and header clearance", (t) => {
  const h = mountChat(t);
  assert.equal(find(h.render(), ChatJumpNav), undefined);
  const messages = history();
  messages[2] = {
    ...messages[2], text: "", attachments: [{ kind: "uploaded_asset", file_name: "photo.png", asset_id: "a", size: 1 }],
  };
  assert.equal(find(h.render({ messages: messages.slice(0, 2) }), ChatJumpNav), undefined);
  let tree = h.render({ messages, loadingHistory: false });
  const nav = find(tree, ChatJumpNav);
  assert.deepEqual(nav.props.anchors, [
    { id: "first", index: 1, title: "First question" },
    { id: "second", index: 2, title: "photo.png" },
  ]);
  const row = h.row(tree, "second");
  let renderedRow = row.render();
  assert.equal(find(renderedRow, visibleContext).props.value, false);
  renderedRow.props.onLayout(layout(620));
  renderedRow.props.onLayout(layout(700));
  h.scrollView.onContentSizeChange();
  nav.props.onJump("second");
  h.flushFrames();
  assert.deepEqual(h.calls, [["jump", { y: 600, animated: false }]]);
  h.scrollView.onScroll(scroll(600));
  tree = h.render();
  renderedRow = row.render(nodes(tree, (item) => item.props.rowID === "second")[0]);
  assert.equal(find(renderedRow, visibleContext).props.value, true);
  assert.ok(returnToBottom(tree));
});

test("an unmeasured jump waits, pauses follow-tail and leaves return-to-bottom working", (t) => {
  const h = mountChat(t);
  let tree = h.render({ messages: history() });
  h.scrollView.onLayout(layout(0, 500));
  h.scrollView.onScroll(scroll(500));
  h.scrollView.onContentSizeChange();
  h.flushFrames();
  assert.deepEqual(h.calls.splice(0), [["tail", { animated: true }]]);
  const nav = find(tree, ChatJumpNav);
  nav.props.onToggle();
  tree = h.render();
  assert.equal(find(tree, ChatJumpNav).props.open, true);
  find(tree, ChatJumpNav).props.onJump("first");
  h.scrollView.onContentSizeChange();
  h.scrollView.onLayout(layout(0, 350));
  keyboardListeners.forEach((listener) => listener());
  h.flushFrames();
  assert.deepEqual(h.calls, []);
  h.row(tree, "second").render().props.onLayout(layout(400));
  assert.deepEqual(h.calls, []);

  h.row(tree, "first").render().props.onLayout(layout(85));
  assert.deepEqual(h.calls.splice(0), [["jump", { y: 0, animated: false }]]);
  h.scrollView.onScroll(scroll(0, 350, 370));
  h.scrollView.onContentSizeChange();
  h.flushFrames();
  assert.deepEqual(h.calls, []);
  tree = h.render();
  assert.equal(find(tree, ChatJumpNav).props.open, false);
  returnToBottom(tree).props.onPress();
  h.flushFrames();
  h.scrollView.onScroll(scroll(500));
  h.scrollView.onContentSizeChange();
  h.flushFrames();
  assert.deepEqual(h.calls, [["tail", { animated: true }], ["tail", { animated: true }]]);
  assert.equal(returnToBottom(h.render()), undefined);
});

test("session switches discard pending jumps and old layouts even when rendered IDs match", (t) => {
  const h = mountChat(t);
  const oldTree = h.render({ messages: history() });
  const oldNav = find(oldTree, ChatJumpNav);
  const oldRow = h.row(oldTree, "second").render();
  const oldScroll = h.sharedRef.current;
  oldNav.props.onJump("second");
  oldNav.props.onToggle();
  assert.equal(find(h.render(), ChatJumpNav).props.open, true);
  const newTree = h.render({ sessionID: "s2", messages: history() });
  assert.notEqual(h.sharedRef.current, oldScroll);
  assert.equal(find(newTree, ChatJumpNav).props.open, false);
  assert.equal(returnToBottom(newTree), undefined);
  const newNav = find(newTree, ChatJumpNav);
  newNav.props.onJump("second");
  oldRow.props.onLayout(layout(900));
  assert.deepEqual(h.calls, []);
  assert.deepEqual(oldScroll.calls, []);
  h.row(newTree, "second").render().props.onLayout(layout(400));
  assert.deepEqual(h.calls, [["jump", { y: 300, animated: false }]]);
});

test("web remeasures the selected row and rejects superseded measurement callbacks", (t) => {
  const h = mountChat(t, "web");
  const tree = h.render({ messages: history() });
  const callbacks = [];
  const rowHost = { measureLayout(content, onSuccess) {
    assert.equal(content, h.content);
    callbacks.push(onSuccess);
  } };
  h.row(tree, "second", rowHost).render().props.onLayout(layout(400));
  const nav = find(tree, ChatJumpNav);
  nav.props.onJump("second");
  nav.props.onJump("second");
  callbacks[0](0, 700);
  assert.deepEqual(h.calls, []);
  h.render({ topClearance: 120 });
  callbacks[1](0, 800);
  assert.deepEqual(h.calls, [["jump", { y: 680, animated: false }]]);
  nav.props.onJump("second");
  h.render({ sessionID: "s2", messages: history() });
  callbacks[2](0, 900);
  assert.deepEqual(h.calls, []);
});

test("navigator exposes expanded state, accessible jump targets and a bounded scrollable list", () => {
  platform.OS = "web";
  const component = renderComponent(ChatJumpNav);
  const jumps = [];
  let toggles = 0;
  const props = {
    anchors: history().filter(({ role }) => role === "user").map(({ id, text }, index) => ({ id, index: index + 1, title: text })),
    buttonFeedback: (style) => style, onJump: (id) => jumps.push(id), onToggle: () => toggles++,
    topClearance: 100, viewportHeight: 195,
  };
  const closed = component.render({ ...props, open: false });
  assert.deepEqual(find(closed, "Pressable").props.accessibilityState, { expanded: false });
  assert.equal(find(closed, "ScrollView"), undefined);
  const open = component.render({ ...props, open: true });
  assert.deepEqual(find(open, "Pressable").props.accessibilityState, { expanded: true });
  const panel = find(open, "AnimatedView");
  assert.equal(panel.props.style[0].width, "100%");
  assert.equal(panel.props.style[1].maxHeight, 96);
  const list = find(open, "ScrollView");
  assert.equal(list.props.keyboardShouldPersistTaps, "handled");
  const entry = nodes(list, (node) => node.props.accessibilityLabel === "Q2，Second question")[0];
  entry.props.onPress();
  assert.deepEqual(jumps, ["second"]);
  open.props.onKeyDown({ key: "Escape", stopPropagation: noop });
  assert.equal(toggles, 1);
});
