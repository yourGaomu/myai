const state = {
  socket: null,
  connected: false,
  activeRequestID: "",
  activeAssistant: null,
  pendingPermission: null,
  permissionQueue: [],
  pendingQuestions: [],
  resolvedQuestionIDs: new Set(),
  pendingSessionRequestID: "",
  sessions: [],
  clientToken: localStorage.getItem("myai_client_token") || "",
  reconnectAttempt: 0,
  reconnectTimer: null,
  heartbeatTimer: null,
};

const el = {
  connectionText: document.querySelector("#connectionText"),
  requestText: document.querySelector("#requestText"),
  refreshAgents: document.querySelector("#refreshAgents"),
  agentList: document.querySelector("#agentList"),
  pairForm: document.querySelector("#pairForm"),
  bindCode: document.querySelector("#bindCode"),
  pairButton: document.querySelector("#pairButton"),
  pairText: document.querySelector("#pairText"),
  refreshSessions: document.querySelector("#refreshSessions"),
  newSession: document.querySelector("#newSession"),
  sessionList: document.querySelector("#sessionList"),
  refreshAuth: document.querySelector("#refreshAuth"),
  authList: document.querySelector("#authList"),
  userId: document.querySelector("#userId"),
  deviceId: document.querySelector("#deviceId"),
  sessionId: document.querySelector("#sessionId"),
  connectButton: document.querySelector("#connectButton"),
  messages: document.querySelector("#messages"),
  composer: document.querySelector("#composer"),
  messageInput: document.querySelector("#messageInput"),
  sendButton: document.querySelector("#sendButton"),
  permissionBox: document.querySelector("#permissionBox"),
  permissionTitle: document.querySelector("#permissionTitle"),
  permissionArgs: document.querySelector("#permissionArgs"),
  allowPermission: document.querySelector("#allowPermission"),
  denyPermission: document.querySelector("#denyPermission"),
  userQuestionBox: document.querySelector("#userQuestionBox"),
  userQuestionTitle: document.querySelector("#userQuestionTitle"),
  userQuestionSource: document.querySelector("#userQuestionSource"),
  userQuestionContent: document.querySelector("#userQuestionContent"),
  dismissUserQuestion: document.querySelector("#dismissUserQuestion"),
  submitUserQuestion: document.querySelector("#submitUserQuestion"),
};

function websocketURL() {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}/ws/client`;
}

function setConnected(connected) {
  state.connected = connected;
  el.connectionText.textContent = connected ? "Connected" : "Disconnected";
  el.connectButton.textContent = connected ? "Reconnect" : "Connect";
  el.sendButton.disabled = !connected;
  renderSessions(state.sessions);
}

function connect() {
  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }
  if (state.socket) {
    state.socket.close();
  }

  const socket = new WebSocket(websocketURL());
  state.socket = socket;
  el.connectionText.textContent = "Connecting";

  socket.addEventListener("open", () => {
    if (state.socket !== socket) {
      return;
    }
    state.reconnectAttempt = 0;
    setConnected(true);
    requestPendingUserQuestions();
    startHeartbeat(socket);
    requestSessions();
  });
  socket.addEventListener("close", () => {
    if (state.socket !== socket) {
      return;
    }
    state.socket = null;
    stopHeartbeat();
    setConnected(false);
    scheduleReconnect();
  });
  socket.addEventListener("error", () => {
    if (state.socket === socket) {
      addMessage("error", "WebSocket connection error; reconnecting automatically");
    }
  });
  socket.addEventListener("message", (event) => {
    try {
      handleMessage(JSON.parse(event.data));
    } catch (err) {
      addMessage("error", `Invalid relay message: ${err.message}`);
    }
  });
}

function startHeartbeat(socket) {
  stopHeartbeat();
  const sendHeartbeat = () => {
    if (state.socket !== socket || socket.readyState !== WebSocket.OPEN || !state.clientToken) {
      return;
    }
    socket.send(JSON.stringify({
      type: "heartbeat",
      user_id: el.userId.value.trim(),
      device_id: el.deviceId.value.trim(),
      client_token: state.clientToken,
      payload: { time: new Date().toISOString() },
    }));
  };
  sendHeartbeat();
  state.heartbeatTimer = setInterval(sendHeartbeat, 25000);
}

function stopHeartbeat() {
  if (state.heartbeatTimer) {
    clearInterval(state.heartbeatTimer);
    state.heartbeatTimer = null;
  }
}

function scheduleReconnect() {
  if (state.reconnectTimer) {
    return;
  }
  const delay = Math.min(30000, 500 * 2 ** state.reconnectAttempt);
  state.reconnectAttempt = Math.min(state.reconnectAttempt + 1, 6);
  el.connectionText.textContent = "Reconnecting";
  state.reconnectTimer = setTimeout(() => {
    state.reconnectTimer = null;
    connect();
  }, delay);
}

async function loadAgents() {
  if (!state.clientToken) {
    renderAgents([]);
    return;
  }
  try {
    const params = new URLSearchParams({
      user_id: el.userId.value.trim(),
      device_id: el.deviceId.value.trim(),
    });
    const response = await fetch(`/agents?${params.toString()}`, {
      headers: authorizationHeaders(),
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text.trim() || response.statusText);
    }
    const data = await response.json();
    renderAgents(data.agents || []);
  } catch (err) {
    renderAgents([]);
    addMessage("error", `Load agents failed: ${err.message}`);
  }
}

async function pairDevice(bindCode) {
  el.pairButton.disabled = true;
  el.pairText.textContent = "Pairing";

  try {
    const response = await fetch("/pair", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bind_code: bindCode, client_name: browserName() }),
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text.trim() || response.statusText);
    }

    const data = await response.json();
    el.userId.value = data.user_id || "";
    el.deviceId.value = data.device_id || "";
    state.clientToken = data.client_token || "";
    localStorage.setItem("myai_client_token", state.clientToken);
    localStorage.setItem("myai_user_id", el.userId.value);
    localStorage.setItem("myai_device_id", el.deviceId.value);
    el.pairText.textContent = `Paired ${el.userId.value}/${el.deviceId.value}`;
    el.bindCode.value = "";
    // Pairing can happen after the initial unauthenticated socket was opened.
    // Rebind that socket so the periodic authenticated heartbeat starts
    // immediately and the Relay can reclaim in-flight requests after a retry.
    if (state.socket && state.socket.readyState === WebSocket.OPEN) {
      startHeartbeat(state.socket);
      requestPendingUserQuestions();
    } else {
      connect();
    }
    await loadAgents();
    await loadAuthorizations();
    requestSessions();
  } catch (err) {
    el.pairText.textContent = err.message;
  } finally {
    el.pairButton.disabled = false;
  }
}

async function loadAuthorizations() {
  el.authList.innerHTML = "";

  if (!state.clientToken) {
    const empty = document.createElement("p");
    empty.textContent = "Pair first";
    el.authList.appendChild(empty);
    return;
  }

  try {
    const params = new URLSearchParams({
      user_id: el.userId.value.trim(),
      device_id: el.deviceId.value.trim(),
    });
    const response = await fetch(`/authorizations?${params.toString()}`, {
      headers: authorizationHeaders(),
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text.trim() || response.statusText);
    }

    const data = await response.json();
    renderAuthorizations(data.authorizations || []);
  } catch (err) {
    renderAuthorizations([]);
    el.pairText.textContent = err.message;
    if (err.message.includes("client token is invalid or expired")) {
      clearPairing("Pairing expired. Pair this browser again");
    }
  }
}

function renderAuthorizations(authorizations) {
  el.authList.innerHTML = "";

  if (authorizations.length === 0) {
    const empty = document.createElement("p");
    empty.textContent = "No active access";
    el.authList.appendChild(empty);
    return;
  }

  authorizations.forEach((authorization) => {
    const item = document.createElement("div");
    item.className = "auth-item";

    const title = document.createElement("strong");
    title.textContent = authorization.current
      ? `${authorization.client_name || "Browser"} (current)`
      : authorization.client_name || "Browser";

    const meta = document.createElement("span");
    meta.textContent = `Last seen ${formatTime(authorization.last_seen_at)} / expires ${formatDate(authorization.expires_at)}`;

    const actions = document.createElement("div");
    actions.className = "auth-actions";
    const revoke = document.createElement("button");
    revoke.type = "button";
    revoke.className = "secondary danger";
    revoke.textContent = "Revoke";
    revoke.addEventListener("click", () => revokeAuthorization(authorization));
    actions.appendChild(revoke);

    item.append(title, meta, actions);
    el.authList.appendChild(item);
  });
}

async function revokeAuthorization(authorization) {
  if (!state.clientToken) {
    return;
  }

  try {
    const response = await fetch("/authorizations/revoke", {
      method: "POST",
      headers: { ...authorizationHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({
        id: authorization.id,
        user_id: el.userId.value.trim(),
        device_id: el.deviceId.value.trim(),
        client_token: state.clientToken,
      }),
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text.trim() || response.statusText);
    }

    if (authorization.current) {
      clearPairing("Current browser access revoked");
      return;
    }
    await loadAuthorizations();
  } catch (err) {
    addMessage("error", `Revoke access failed: ${err.message}`);
  }
}

function renderAgents(agents) {
  el.agentList.innerHTML = "";

  if (agents.length === 0) {
    const empty = document.createElement("p");
    empty.textContent = "No agents online";
    el.agentList.appendChild(empty);
    return;
  }

  agents.forEach((agent) => {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "agent-item";
    if (agent.user_id === el.userId.value && agent.device_id === el.deviceId.value) {
      item.classList.add("active");
    }

    const title = document.createElement("strong");
    title.textContent = `${agent.user_id}/${agent.device_id}`;
    const meta = document.createElement("span");
    meta.textContent = `Last seen ${formatTime(agent.last_seen_at)}`;

    item.append(title, meta);
    item.addEventListener("click", () => {
      const wasPairedTarget =
        state.clientToken && agent.user_id === el.userId.value && agent.device_id === el.deviceId.value;
      el.userId.value = agent.user_id;
      el.deviceId.value = agent.device_id;
      if (!wasPairedTarget) {
        clearPairing("Pair required for selected agent");
      }
      renderAgents(agents);
    });
    el.agentList.appendChild(item);
  });
}

function formatTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString();
}

function formatDate(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleDateString();
}

function authorizationHeaders() {
  return { Authorization: `Bearer ${state.clientToken}` };
}

function browserName() {
  const platform = navigator.platform || "unknown";
  return `Browser on ${platform}`;
}

function sendUserMessage(content) {
  if (!state.clientToken) {
    addMessage("error", "Please pair this browser with an agent first");
    return;
  }

  state.activeRequestID = newRequestID();
  state.activeAssistant = null;
  hidePermission(true);

  addMessage("user", content);
  el.requestText.textContent = `Request ${state.activeRequestID}`;

  sendEnvelope("user_message", {
    request_id: state.activeRequestID,
    payload: { content },
  });
}

function requestSessions() {
  if (!state.clientToken) {
    renderSessions([]);
    return;
  }

  state.pendingSessionRequestID = newRequestID();
  sendEnvelope("session_list", {
    request_id: state.pendingSessionRequestID,
  });
}

function newRemoteSession() {
  if (!state.clientToken) {
    addMessage("error", "Please pair this browser with an agent first");
    return;
  }

  state.pendingSessionRequestID = newRequestID();
  sendEnvelope("session_new", {
    request_id: state.pendingSessionRequestID,
  });
}

function loadRemoteSession(sessionID) {
  if (!sessionID) {
    return;
  }
  if (!state.clientToken) {
    addMessage("error", "Please pair this browser with an agent first");
    return;
  }

  state.pendingSessionRequestID = newRequestID();
  sendEnvelope("session_load", {
    request_id: state.pendingSessionRequestID,
    session_id: sessionID,
    payload: { session_id: sessionID },
  });
}

function sendPermissionResult(allowed) {
  if (!state.pendingPermission) {
    return;
  }

  const sent = sendEnvelope("permission_result", {
    request_id: state.pendingPermission.requestID,
    session_id: state.pendingPermission.sessionID,
    payload: { allowed, approval_id: state.pendingPermission.approvalID },
  });
  if (!sent) {
    return;
  }
  addMessage("event", `${allowed ? "Allowed" : "Denied"} ${state.pendingPermission.name}`);
  hidePermission();
}

function sendEnvelope(type, overrides) {
  if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
    addMessage("error", "WebSocket is not connected");
    return false;
  }

  const message = {
    type,
    request_id: overrides.request_id || state.activeRequestID,
    user_id: el.userId.value.trim(),
    device_id: el.deviceId.value.trim(),
    session_id: overrides.session_id || el.sessionId.value.trim(),
    client_token: state.clientToken,
    payload: overrides.payload || {},
  };
  try {
    state.socket.send(JSON.stringify(message));
    return true;
  } catch (err) {
    addMessage("error", `WebSocket send failed: ${err.message}`);
    state.socket.close();
    return false;
  }
}

function handleMessage(message) {
  if (!shouldHandleMessage(message)) {
    return;
  }

  switch (message.type) {
    case "heartbeat":
      el.requestText.textContent = `Ack ${message.request_id || ""}`.trim();
      break;
    case "assistant_delta":
      appendAssistant(readPayload(message).content || "");
      break;
    case "assistant_done":
      el.requestText.textContent = "Done";
      if (message.session_id) {
        el.sessionId.value = message.session_id;
      }
      clearPermissionsForRequest(message.request_id);
      break;
    case "tool_call": {
      const payload = readPayload(message);
      addMessage("event", `Tool call: ${payload.name || ""}\n${payload.arguments || ""}`);
      break;
    }
    case "tool_result": {
      const payload = readPayload(message);
      addMessage(payload.error ? "error" : "event", `Tool ${payload.error ? "error" : "result"}: ${payload.name || ""}\n${payload.result || ""}`);
      break;
    }
    case "permission_ask":
      showPermission(message);
      break;
    case "user_question":
      showUserQuestion(message);
      break;
    case "user_question_resolved": {
      const payload = readPayload(message);
      rememberResolvedQuestion(payload.question_id);
      state.pendingQuestions = state.pendingQuestions.filter((item) => item.questionID !== payload.question_id);
      renderUserQuestion();
      if (payload.status && payload.status !== "answered") {
        addMessage("event", `User question ended: ${payload.status}`);
      }
      break;
    }
    case "user_question_list_result": {
      const payload = readPayload(message);
      for (const questionID of payload.resolved_question_ids || []) {
        rememberResolvedQuestion(questionID);
      }
      state.pendingQuestions = state.pendingQuestions.filter((item) => !state.resolvedQuestionIDs.has(item.questionID));
      for (const item of payload.questions || []) {
        const question = item.question || {};
        if (state.resolvedQuestionIDs.has(question.question_id)) continue;
        showUserQuestion({
          request_id: item.request_id || "",
          session_id: item.session_id || "",
          payload: question,
        });
      }
      renderUserQuestion();
      break;
    }
    case "session_list_result":
      applySessionList(readPayload(message));
      break;
    case "session_changed":
      applySessionChanged(readPayload(message));
      addMessage("event", `Session ${shortID(el.sessionId.value)} selected`);
      break;
    case "error": {
      const errorText = readPayload(message).message || "Remote error";
      addMessage("error", errorText);
      if (errorText.includes("client token is invalid or expired")) {
        clearPairing("Pairing expired. Pair this browser again");
      }
      clearPermissionsForRequest(message.request_id);
      break;
    }
    default:
      addMessage("event", `Message: ${message.type}`);
  }
}

function requestPendingUserQuestions() {
  if (!state.clientToken || !state.connected) {
    return;
  }
  sendEnvelope("user_question_list", {
    request_id: newRequestID(),
    payload: { known_question_ids: state.pendingQuestions.map((item) => item.questionID) },
  });
}

function readPayload(message) {
  return message.payload || {};
}

function isSessionMessage(type) {
  return type === "session_list_result" || type === "session_changed";
}

function isPassiveMessage(type) {
  return isSessionMessage(type) || type === "user_question_list_result";
}

function isTrackedRequest(requestID) {
  return (
    requestID === state.activeRequestID ||
    requestID === state.pendingSessionRequestID
  );
}

function shouldHandleMessage(message) {
  if (!message.request_id) {
    return true;
  }
  if (isPassiveMessage(message.type)) {
    return true;
  }
  if (message.type === "error") {
    return isTrackedRequest(message.request_id) || !state.activeRequestID;
  }
  if (!state.activeRequestID) {
    return true;
  }
  return isTrackedRequest(message.request_id);
}

function applySessionList(payload) {
  state.sessions = payload.sessions || [];
  if (payload.current_session_id) {
    el.sessionId.value = payload.current_session_id;
  }
  renderSessions(state.sessions);
}

function applySessionChanged(payload) {
  state.sessions = payload.sessions || [];
  if (payload.current_session_id) {
    el.sessionId.value = payload.current_session_id;
  } else if (payload.session && payload.session.id) {
    el.sessionId.value = payload.session.id;
  }
  renderSessions(state.sessions);
}

function renderSessions(sessions) {
  el.sessionList.innerHTML = "";

  if (!state.clientToken) {
    const empty = document.createElement("p");
    empty.textContent = "Pair first";
    el.sessionList.appendChild(empty);
    return;
  }

  if (!state.connected) {
    const empty = document.createElement("p");
    empty.textContent = "Connect first";
    el.sessionList.appendChild(empty);
    return;
  }

  if (sessions.length === 0) {
    const empty = document.createElement("p");
    empty.textContent = "No sessions loaded";
    el.sessionList.appendChild(empty);
    return;
  }

  sessions.forEach((session) => {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "session-item";
    if (session.id === el.sessionId.value) {
      item.classList.add("active");
    }

    const title = document.createElement("strong");
    title.textContent = session.title || "New chat";

    const meta = document.createElement("span");
    meta.textContent = `${shortID(session.id)} / ${session.model || "model"} / ${formatTime(session.updated_at)}`;

    item.append(title, meta);
    item.addEventListener("click", () => loadRemoteSession(session.id));
    el.sessionList.appendChild(item);
  });
}

function appendAssistant(text) {
  if (!state.activeAssistant) {
    state.activeAssistant = addMessage("assistant", "");
  }
  state.activeAssistant.textContent += text;
  el.messages.scrollTop = el.messages.scrollHeight;
}

function addMessage(kind, text) {
  const node = document.createElement("div");
  node.className = `message ${kind}`;
  node.textContent = text;
  el.messages.appendChild(node);
  el.messages.scrollTop = el.messages.scrollHeight;
  return node;
}

function showPermission(message) {
  const payload = readPayload(message);
  if (!payload.approval_id) {
    addMessage("error", "Permission request is missing approval_id");
    return;
  }
  const permission = {
    approvalID: payload.approval_id,
    requestID: message.request_id,
    sessionID: message.session_id || "",
    name: payload.name || "tool",
    permission: payload.permission || "permission",
    arguments: payload.arguments || "",
  };
  if (state.pendingPermission) {
    state.permissionQueue.push(permission);
    return;
  }
  displayPermission(permission);
}

function displayPermission(permission) {
  state.pendingPermission = permission;
  el.permissionTitle.textContent = `${permission.name} requires ${permission.permission}`;
  el.permissionArgs.textContent = permission.arguments;
  el.permissionBox.classList.remove("hidden");
}

function hidePermission(clearAll = false) {
  if (clearAll) {
    state.permissionQueue = [];
  }
  state.pendingPermission = null;
  el.permissionBox.classList.add("hidden");
  if (state.permissionQueue.length > 0) {
    displayPermission(state.permissionQueue.shift());
  }
}

function clearPermissionsForRequest(requestID) {
  state.permissionQueue = state.permissionQueue.filter((item) => item.requestID !== requestID);
  if (state.pendingPermission?.requestID === requestID) {
    hidePermission();
  }
}

function showUserQuestion(message) {
  const payload = readPayload(message);
  if (!payload.question_id || !Array.isArray(payload.questions) || payload.questions.length === 0) {
    addMessage("error", "User question payload is invalid");
    return;
  }
  if (state.resolvedQuestionIDs.has(payload.question_id)) {
    return;
  }
  const existing = state.pendingQuestions.find((item) => item.questionID === payload.question_id);
  if (existing) {
    existing.requestID = message.request_id || existing.requestID;
    existing.sessionID = message.session_id || existing.sessionID;
    existing.title = payload.title || existing.title;
    existing.sourceTaskID = payload.source_task_id || existing.sourceTaskID;
    existing.questions = payload.questions;
    renderUserQuestion();
    return;
  }
  state.pendingQuestions.push({
    questionID: payload.question_id,
    requestID: message.request_id || "",
    sessionID: message.session_id || "",
    title: payload.title || "The Agent needs a clarification",
    sourceTaskID: payload.source_task_id || "",
    questions: payload.questions,
    submitted: false,
  });
  renderUserQuestion();
}

function rememberResolvedQuestion(questionID) {
  if (!questionID) {
    return;
  }
  state.resolvedQuestionIDs.add(questionID);
  if (state.resolvedQuestionIDs.size > 256) {
    const oldest = state.resolvedQuestionIDs.values().next().value;
    if (oldest) state.resolvedQuestionIDs.delete(oldest);
  }
}

function renderUserQuestion() {
  const question = state.pendingQuestions[0];
  if (!question) {
    el.userQuestionBox.classList.add("hidden");
    el.userQuestionContent.replaceChildren();
    return;
  }
  el.userQuestionTitle.textContent = question.title;
  el.userQuestionSource.textContent = question.sourceTaskID ? `Subagent ${shortID(question.sourceTaskID)}` : "";
  el.userQuestionContent.replaceChildren();
  question.questions.forEach((item, index) => {
    const group = document.createElement("fieldset");
    group.className = "user-question-item";
    const legend = document.createElement("legend");
    legend.textContent = item.prompt;
    group.appendChild(legend);
    const name = `user-question-${index}`;
    const addChoice = (label, value) => {
      const wrapper = document.createElement("label");
      wrapper.className = "user-question-choice";
      const radio = document.createElement("input");
      radio.type = "radio";
      radio.name = name;
      radio.value = value;
      radio.disabled = question.submitted;
      wrapper.append(radio, document.createTextNode(label));
      group.appendChild(wrapper);
    };
    (item.options || []).forEach((option) => addChoice(option, option));
    if (item.allow_other || !item.options?.length) {
      const wrapper = document.createElement("label");
      wrapper.className = "user-question-choice";
      if (item.options?.length) {
        const radio = document.createElement("input");
        radio.type = "radio";
        radio.name = name;
        radio.value = "__other__";
        radio.disabled = question.submitted;
        wrapper.append(radio, document.createTextNode("Other"));
      }
      const input = document.createElement("input");
      input.type = "text";
      input.dataset.otherQuestionIndex = String(index);
      input.placeholder = item.options?.length ? "Other answer" : "Type your answer";
      input.disabled = question.submitted;
      input.addEventListener("input", () => {
        if (input.value.trim()) {
          const radio = group.querySelector(`input[name="${name}"][value="__other__"]`);
          if (radio) radio.checked = true;
        }
      });
      wrapper.appendChild(input);
      group.appendChild(wrapper);
    }
    el.userQuestionContent.appendChild(group);
  });
  el.submitUserQuestion.disabled = question.submitted;
  el.dismissUserQuestion.disabled = question.submitted;
  el.submitUserQuestion.textContent = question.submitted ? "已提交" : "提交回答";
  el.userQuestionBox.classList.remove("hidden");
}

function submitUserQuestion(cancelled = false) {
  const question = state.pendingQuestions[0];
  if (!question || question.submitted) {
    return;
  }
  const answers = {};
  if (!cancelled) {
    for (let index = 0; index < question.questions.length; index += 1) {
      const item = question.questions[index];
      const name = `user-question-${index}`;
      const selected = el.userQuestionContent.querySelector(`input[name="${name}"]:checked`);
      if (selected?.value === "__other__") {
        answers[item.id] = el.userQuestionContent.querySelector(`[data-other-question-index="${index}"]`)?.value.trim() || "";
      } else if (selected) {
        answers[item.id] = selected.value;
      } else if (!item.options?.length) {
        answers[item.id] = el.userQuestionContent.querySelector(`[data-other-question-index="${index}"]`)?.value.trim() || "";
      }
      if (!answers[item.id]) {
        addMessage("error", "Please answer each question before submitting");
        return;
      }
    }
  }
  const sent = sendEnvelope("user_question_answer", {
    request_id: question.requestID,
    session_id: question.sessionID,
    payload: { question_id: question.questionID, answers, cancelled },
  });
  if (sent) {
    question.submitted = true;
    renderUserQuestion();
  }
}

function newRequestID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function shortID(value) {
  if (!value) {
    return "-";
  }
  return value.length > 8 ? value.slice(0, 8) : value;
}

function restorePairing() {
  const userID = localStorage.getItem("myai_user_id") || "";
  const deviceID = localStorage.getItem("myai_device_id") || "";
  if (userID) {
    el.userId.value = userID;
  }
  if (deviceID) {
    el.deviceId.value = deviceID;
  }
  if (state.clientToken && el.userId.value && el.deviceId.value) {
    el.pairText.textContent = `Paired ${el.userId.value}/${el.deviceId.value}`;
  }
}

function clearPairing(text) {
  state.clientToken = "";
  state.sessions = [];
  state.pendingQuestions = [];
  state.resolvedQuestionIDs.clear();
  renderUserQuestion();
  localStorage.removeItem("myai_client_token");
  el.pairText.textContent = text;
  el.authList.innerHTML = "";
  const empty = document.createElement("p");
  empty.textContent = "Pair first";
  el.authList.appendChild(empty);
  renderSessions([]);
}

el.connectButton.addEventListener("click", connect);
el.refreshAgents.addEventListener("click", loadAgents);
el.refreshSessions.addEventListener("click", requestSessions);
el.newSession.addEventListener("click", newRemoteSession);
el.refreshAuth.addEventListener("click", loadAuthorizations);
el.allowPermission.addEventListener("click", () => sendPermissionResult(true));
el.denyPermission.addEventListener("click", () => sendPermissionResult(false));
el.submitUserQuestion.addEventListener("click", () => submitUserQuestion());
el.dismissUserQuestion.addEventListener("click", () => submitUserQuestion(true));
el.userId.addEventListener("change", () => clearPairing("Pair required for edited target"));
el.deviceId.addEventListener("change", () => clearPairing("Pair required for edited target"));

el.pairForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const bindCode = el.bindCode.value.trim();
  if (!bindCode) {
    el.pairText.textContent = "Bind code is empty";
    return;
  }
  pairDevice(bindCode);
});

el.composer.addEventListener("submit", (event) => {
  event.preventDefault();
  const content = el.messageInput.value.trim();
  if (!content) {
    return;
  }
  sendUserMessage(content);
  el.messageInput.value = "";
});

el.messageInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
    el.composer.requestSubmit();
  }
});

setConnected(false);
restorePairing();
connect();
loadAgents();
loadAuthorizations();
setInterval(loadAgents, 5000);
