const state = {
  socket: null,
  connected: false,
  activeRequestID: "",
  activeAssistant: null,
  pendingPermission: null,
  clientToken: localStorage.getItem("myai_client_token") || "",
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
}

function connect() {
  if (state.socket) {
    state.socket.close();
  }

  const socket = new WebSocket(websocketURL());
  state.socket = socket;
  el.connectionText.textContent = "Connecting";

  socket.addEventListener("open", () => setConnected(true));
  socket.addEventListener("close", () => setConnected(false));
  socket.addEventListener("error", () => addMessage("error", "WebSocket connection error"));
  socket.addEventListener("message", (event) => {
    try {
      handleMessage(JSON.parse(event.data));
    } catch (err) {
      addMessage("error", `Invalid relay message: ${err.message}`);
    }
  });
}

async function loadAgents() {
  try {
    const response = await fetch("/agents");
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
    await loadAgents();
    await loadAuthorizations();
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
    meta.textContent = `Last seen ${formatTime(authorization.last_seen_at)} · expires ${formatDate(authorization.expires_at)}`;

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
  hidePermission();

  addMessage("user", content);
  el.requestText.textContent = `Request ${state.activeRequestID}`;

  sendEnvelope("user_message", {
    request_id: state.activeRequestID,
    payload: { content },
  });
}

function sendPermissionResult(allowed) {
  if (!state.pendingPermission) {
    return;
  }

  sendEnvelope("permission_result", {
    request_id: state.pendingPermission.requestID,
    session_id: state.pendingPermission.sessionID,
    payload: { allowed },
  });
  addMessage("event", `${allowed ? "Allowed" : "Denied"} ${state.pendingPermission.name}`);
  hidePermission();
}

function sendEnvelope(type, overrides) {
  if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
    addMessage("error", "WebSocket is not connected");
    return;
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
  state.socket.send(JSON.stringify(message));
}

function handleMessage(message) {
  if (message.request_id && state.activeRequestID && message.request_id !== state.activeRequestID) {
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
      hidePermission();
      break;
    case "tool_call": {
      const payload = readPayload(message);
      addMessage("event", `Tool call: ${payload.name || ""}\n${payload.arguments || ""}`);
      break;
    }
    case "permission_ask":
      showPermission(message);
      break;
    case "error": {
      const errorText = readPayload(message).message || "Remote error";
      addMessage("error", errorText);
      if (errorText.includes("client token is invalid or expired")) {
        clearPairing("Pairing expired. Pair this browser again");
      }
      hidePermission();
      break;
    }
    default:
      addMessage("event", `Message: ${message.type}`);
  }
}

function readPayload(message) {
  return message.payload || {};
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
  state.pendingPermission = {
    requestID: message.request_id,
    sessionID: message.session_id || "",
    name: payload.name || "tool",
  };
  el.permissionTitle.textContent = `${payload.name || "Tool"} requires ${payload.permission || "permission"}`;
  el.permissionArgs.textContent = payload.arguments || "";
  el.permissionBox.classList.remove("hidden");
}

function hidePermission() {
  state.pendingPermission = null;
  el.permissionBox.classList.add("hidden");
}

function newRequestID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
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
  localStorage.removeItem("myai_client_token");
  el.pairText.textContent = text;
  el.authList.innerHTML = "";
  const empty = document.createElement("p");
  empty.textContent = "Pair first";
  el.authList.appendChild(empty);
}

el.connectButton.addEventListener("click", connect);
el.refreshAgents.addEventListener("click", loadAgents);
el.refreshAuth.addEventListener("click", loadAuthorizations);
el.allowPermission.addEventListener("click", () => sendPermissionResult(true));
el.denyPermission.addEventListener("click", () => sendPermissionResult(false));
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
