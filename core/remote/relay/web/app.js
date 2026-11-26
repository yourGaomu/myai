const state = {
  socket: null,
  connected: false,
  activeRequestID: "",
  activeAssistant: null,
  jevTraces: new Map(),
  jevRequests: new Map(),
  pendingPermission: null,
  permissionQueue: [],
  pendingQuestions: [],
  pendingQuestionList: null,
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
  adoptUserQuestion: document.querySelector("#adoptUserQuestion"),
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
  renderUserQuestion();
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
    if (state.socket !== socket) return;
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
    case "intent_trace_event":
      updateJevTrace(readPayload(message));
      break;
    case "intent_trace_get_result": {
      const traceID = state.jevRequests.get(message.request_id);
      state.jevRequests.delete(message.request_id);
      const trace = readPayload(message).trace;
      if (traceID && trace?.trace_id === traceID) updateJevTrace(trace);
      break;
    }
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
    case "user_question_resolved":
      resolveUserQuestion(readPayload(message).question_id, readPayload(message).status);
      break;
    case "background_turn_event": {
      const payload = readPayload(message);
      if (payload.kind === "intent_trace" && payload.session_id === el.sessionId.value.trim()) {
        updateJevTrace(payload.intent_trace);
      } else if (payload.kind === "user_question") {
        showUserQuestion({ request_id: payload.turn_id, session_id: payload.session_id, payload: payload.question });
      } else if (payload.kind === "user_question_resolved") {
        resolveUserQuestion(payload.question?.question_id, payload.status);
      } else if (payload.kind === "resync_required") {
        requestPendingUserQuestions();
      }
      break;
    }
    case "user_question_list_result": {
      const request = state.pendingQuestionList;
      if (!request || message.request_id !== request.id) break;
      state.pendingQuestionList = null;
      const payload = readPayload(message);
      for (const questionID of payload.resolved_question_ids || []) {
        rememberResolvedQuestion(questionID);
      }
      state.pendingQuestions = state.pendingQuestions.filter((item) => !state.resolvedQuestionIDs.has(item.questionID));
      for (const item of payload.questions || []) {
        showUserQuestion({ request_id: item.request_id, session_id: item.session_id, payload: item.question }, request.submitted);
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
      const traceID = state.jevRequests.get(message.request_id);
      if (traceID) {
        state.jevRequests.delete(message.request_id);
        const entry = state.jevTraces.get(traceID);
        if (entry) {
          entry.requestedStatus = "";
          entry.output.textContent = "Could not load the Jev record. Reopen to retry.";
        }
        break;
      }
      const errorText = readPayload(message).message || "Remote error";
      addMessage("error", errorText);
      if (errorText.includes("client token is invalid or expired")) {
        clearPairing("Pairing expired. Pair this browser again");
      }
      clearPermissionsForRequest(message.request_id);
      if (state.pendingQuestions.some((item) => item.submitted && matchesQuestionRequest(item, message))) {
        requestPendingUserQuestions();
      }
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
  const requestID = newRequestID();
  if (sendEnvelope("user_question_list", {
    request_id: requestID,
    payload: { known_question_ids: state.pendingQuestions.map((item) => item.questionID) },
  })) {
    state.pendingQuestionList = {
      id: requestID,
      submitted: new Set(state.pendingQuestions.filter((item) => item.submitted)),
    };
  }
}

function readPayload(message) {
  return message.payload || {};
}

function isSessionMessage(type) {
  return type === "session_list_result" || type === "session_changed";
}

function isPassiveMessage(type) {
  return isSessionMessage(type) || type === "user_question_list_result" ||
    type === "user_question" || type === "user_question_resolved" || type === "background_turn_event";
}

function isTrackedRequest(requestID) {
  return (
    requestID === state.activeRequestID ||
    requestID === state.pendingSessionRequestID ||
    state.jevRequests.has(requestID)
  );
}

function matchesQuestionRequest(question, message) {
  return question.requestID === message.request_id && question.sessionID === message.session_id;
}

function shouldHandleMessage(message) {
  if (message.type === "user_question" || message.type === "user_question_resolved" ||
      message.type === "user_question_list_result" || message.type === "background_turn_event") {
    if (!state.clientToken || (message.user_id && message.user_id !== el.userId.value.trim()) ||
        (message.device_id && message.device_id !== el.deviceId.value.trim())) return false;
  }
  if (!message.request_id) {
    return true;
  }
  if (isPassiveMessage(message.type)) {
    return true;
  }
  if (message.type === "error") {
    return isTrackedRequest(message.request_id) || !state.activeRequestID ||
      state.pendingQuestions.some((item) => matchesQuestionRequest(item, message));
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

function updateJevTrace(trace) {
  if (!trace?.trace_id) return;
  let entry = state.jevTraces.get(trace.trace_id);
  if (!entry) {
    const node = document.createElement("details");
    node.className = "message event jev-trace";
    const summary = document.createElement("summary");
    const inputTitle = document.createElement("strong");
    inputTitle.textContent = "Input";
    const input = document.createElement("pre");
    const outputTitle = document.createElement("strong");
    outputTitle.textContent = "Output";
    const output = document.createElement("pre");
    node.append(summary, inputTitle, input, outputTitle, output);
    entry = { node, summary, input, output, trace, requestedStatus: "" };
    node.addEventListener("toggle", () => requestJevTraceDetails(entry));
    state.jevTraces.set(trace.trace_id, entry);
    el.messages.appendChild(node);
    el.messages.scrollTop = el.messages.scrollHeight;
  }
  if (entry.trace.status !== "started" && trace.status === "started") return;
  entry.trace = { ...entry.trace, ...trace };
  const value = entry.trace;
  const status = value.status === "started" ? "Checking" : value.status === "succeeded" ? "Complete" : value.status.replaceAll("_", " ");
  const routes = { plan_only: "plan awaits confirmation", plan_resume: "resume plan", chat: "chat" };
  const result = value.choice ? ` · ${value.choice} (${Math.round((value.confidence ?? 0) * 100)}%)` : "";
  const route = value.route ? ` · ${routes[value.route] || value.route}` : "";
  entry.summary.textContent = `Jev intent check · ${status}${result}${route}`;
  entry.input.textContent = prettyJevBody(value.request_body) || "Open to load input";
  entry.output.textContent = prettyJevBody(value.response_body) || (value.status === "started" ? "Waiting for response…" : "No response body");
  if (value.response_truncated) entry.output.textContent += "\nResponse truncated.";
  requestJevTraceDetails(entry);
}

function requestJevTraceDetails(entry) {
  const trace = entry.trace;
  if (!entry.node.open || entry.requestedStatus === trace.status ||
      (trace.request_body && (trace.response_body || trace.status === "started"))) return;
  const requestID = newRequestID();
  if (sendEnvelope("intent_trace_get", { request_id: requestID, payload: { trace_id: trace.trace_id } })) {
    entry.requestedStatus = trace.status;
    state.jevRequests.set(requestID, trace.trace_id);
  }
}

function prettyJevBody(body) {
  if (!body) return "";
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
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

function showUserQuestion(message, submittedAtQuery) {
  const payload = readPayload(message);
  if (!message.request_id || !message.session_id || typeof payload.question_id !== "string" || !payload.question_id ||
      payload.schema_version !== 2 || !Array.isArray(payload.questions) || payload.questions.length === 0 || payload.questions.length > 5) {
    addMessage("error", "问答协议版本不匹配或载荷无效，请同时升级 Agent 与 Relay Web。");
    return;
  }
  const ids = new Set();
  for (const item of payload.questions) {
    if (!item || typeof item.id !== "string" || !item.id.trim() || ids.has(item.id) ||
        typeof item.prompt !== "string" || !item.prompt.trim() || typeof item.multi_select !== "boolean" ||
        typeof item.allow_other !== "boolean" || !Array.isArray(item.choices) || item.choices.length > 8 ||
        (!item.choices.length && !item.allow_other) ||
        item.choices.some((choice) => !choice || typeof choice.label !== "string" || !choice.label.trim())) {
      addMessage("error", "问答协议无效，请同时升级 Agent 与 Relay Web。");
      return;
    }
    ids.add(item.id);
  }
  if (state.resolvedQuestionIDs.has(payload.question_id)) return;
  const existing = state.pendingQuestions.find((item) => item.questionID === payload.question_id);
  if (existing) {
    if (submittedAtQuery?.has(existing) && matchesQuestionRequest(existing, message)) existing.submitted = false;
    return;
  }
  state.pendingQuestions.push({
    questionID: payload.question_id,
    requestID: message.request_id,
    sessionID: message.session_id,
    title: payload.title || "The Agent needs a clarification",
    sourceTaskID: payload.source_task_id || "",
    questions: payload.questions,
    drafts: new Map(),
    submitted: false,
  });
  if (!submittedAtQuery && state.pendingQuestions.length === 1) renderUserQuestion();
}

function resolveUserQuestion(questionID, status) {
  if (!questionID) return;
  const wasPending = state.pendingQuestions.some((item) => item.questionID === questionID);
  rememberResolvedQuestion(questionID);
  if (!wasPending) return;
  state.pendingQuestions = state.pendingQuestions.filter((item) => item.questionID !== questionID);
  renderUserQuestion();
  if (status && status !== "answered") addMessage("event", `User question ended: ${status}`);
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
    group.disabled = question.submitted || !state.connected;
    const legend = document.createElement("legend");
    legend.textContent = `${item.prompt}${item.multi_select ? "（多选）" : ""}`;
    group.appendChild(legend);
    const draft = question.drafts.get(item.id) || { selected: [], other: "" };
    const controls = [];
    const otherInput = item.allow_other ? document.createElement("input") : null;
    const saveDraft = () => {
      question.drafts.set(item.id, {
        selected: controls.filter((input) => input.checked).map((input) => input.value),
        other: otherInput?.value || "",
      });
      updateQuestionActions(question);
    };
    for (const choice of item.choices) {
      const wrapper = document.createElement("label");
      wrapper.className = "user-question-choice";
      const input = document.createElement("input");
      input.type = item.multi_select ? "checkbox" : "radio";
      input.name = `user-question-${index}`;
      input.value = choice.label;
      input.checked = draft.selected.includes(choice.label);
      input.addEventListener("change", () => {
        if (!item.multi_select && otherInput) otherInput.value = "";
        saveDraft();
      });
      controls.push(input);
      wrapper.append(input, document.createTextNode(choice.label));
      if (choice.recommended) {
        const badge = document.createElement("strong");
        badge.className = "user-question-recommended";
        badge.textContent = "推荐";
        wrapper.appendChild(badge);
      }
      group.appendChild(wrapper);
      if (choice.description) {
        const description = document.createElement("small");
        description.className = "user-question-description";
        description.textContent = choice.description;
        group.appendChild(description);
      }
    }
    if (otherInput) {
      otherInput.type = "text";
      otherInput.value = draft.other;
      otherInput.placeholder = item.multi_select ? "补充要求（可与已选项同时生效）" : "输入自定义回答";
      otherInput.addEventListener("input", () => {
        if (!item.multi_select && otherInput.value.trim()) {
          for (const input of controls) input.checked = false;
        }
        saveDraft();
      });
      group.appendChild(otherInput);
    }
    el.userQuestionContent.appendChild(group);
  });
  updateQuestionActions(question);
  el.userQuestionBox.classList.remove("hidden");
}

function updateQuestionActions(question) {
  const unanswered = question.questions.filter((item) => {
    const draft = question.drafts.get(item.id);
    return !draft?.selected.length && !draft?.other.trim();
  });
  const canAdopt = unanswered.length > 0 && unanswered.every((item) => item.choices.some((choice) => choice.recommended));
  const locked = question.submitted || !state.connected;
  el.adoptUserQuestion.classList.toggle("hidden", question.submitted || !canAdopt);
  el.adoptUserQuestion.disabled = locked || !canAdopt;
  el.submitUserQuestion.disabled = locked || unanswered.length > 0;
  el.dismissUserQuestion.disabled = locked;
  el.submitUserQuestion.textContent = question.submitted ? "已提交" : "提交回答";
}

function collectStructuredQuestionAnswers(question, adoptRecommendations = false) {
  return Object.fromEntries(question.questions.map((item) => {
    const draft = question.drafts.get(item.id);
    let selected = draft?.selected || [];
    const other = draft?.other.trim() || "";
    if (adoptRecommendations && !selected.length && !other) {
      selected = item.choices.filter((choice) => choice.recommended).map((choice) => choice.label);
    }
    if (!selected.length && !other) throw new Error("请回答每个问题后再提交。");
    return [item.id, {
      ...(selected.length ? { selected } : {}),
      ...(other ? { other } : {}),
    }];
  }));
}

function submitUserQuestion(cancelled = false, adoptRecommendations = false) {
  const question = state.pendingQuestions[0];
  if (!question || question.submitted || !state.connected) return;
  let structuredAnswers = {};
  if (!cancelled) {
    try { structuredAnswers = collectStructuredQuestionAnswers(question, adoptRecommendations); }
    catch (error) { addMessage("error", error.message); return; }
  }
  const sent = sendEnvelope("user_question_answer", {
    request_id: question.requestID,
    session_id: question.sessionID,
    payload: {
      schema_version: 2,
      question_id: question.questionID,
      structured_answers: structuredAnswers,
      adopted_defaults: adoptRecommendations || undefined,
      cancelled,
    },
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
  for (const entry of state.jevTraces.values()) entry.node.remove();
  state.jevTraces.clear();
  state.jevRequests.clear();
  state.sessions = [];
  state.pendingQuestions = [];
  state.pendingQuestionList = null;
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
el.adoptUserQuestion.addEventListener("click", () => submitUserQuestion(false, true));
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
