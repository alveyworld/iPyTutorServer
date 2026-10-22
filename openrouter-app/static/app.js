const STORAGE_KEY = "ipyintervu_public_key";

const messagesEl = document.getElementById("messages");
const chatForm = document.getElementById("chat-form");
const promptEl = document.getElementById("prompt");
const sendBtn = document.getElementById("send-btn");
const appEl = document.querySelector(".app");
const authGateEl = document.getElementById("auth-gate");
const authFormEl = document.getElementById("auth-form");
const authLoggingInEl = document.getElementById("auth-logging-in");
const authLoggingInTitleEl = document.getElementById("auth-logging-in-title");
const publicKeyInputEl = document.getElementById("public-key-input");
const authErrorEl = document.getElementById("auth-error");
const authSubmitBtn = document.getElementById("auth-submit-btn");
const assessmentTimesEl = document.getElementById("assessment-times");
const assessmentStartEl = document.getElementById("assessment-start-time");
const assessmentEndEl = document.getElementById("assessment-end-time");
const earlyCoachingBannerEl = document.getElementById("early-coaching-banner");

const conversation = [];
let isAuthenticated = false;
// Session token for this page only. The session cookie is shared by every tab,
// so another tab logging in would otherwise switch this page to its session.
let sessionToken = "";

function sessionHeaders(extra = {}) {
  return sessionToken ? { ...extra, [SESSION_TOKEN_HEADER]: sessionToken } : extra;
}
let activeChatAbort = null;
let chatInFlight = false;

const MAX_USER_MESSAGE_CHARS = 5000;
const MAX_CHAT_RETRIES = 5;
const CHAT_RETRY_DELAY_MS = 1500;
const MAX_CHAT_RECOVERY_ATTEMPTS = 4;
const CHAT_READ_TIMEOUT_MS = 120_000;
const BOOTSTRAP_TURN_ID = "ipyintervu-bootstrap";
const CHAT_MODEL = "deepseek/deepseek-v4-flash";
const TURN_ID_HEADER = "X-Turn-Id";
const SESSION_TOKEN_HEADER = "X-Session-Token";
const PROMPT_PLACEHOLDER_IDLE = "Tell me your answer.";
const PROMPT_PLACEHOLDER_WORKING_BASE = "considering your response";
const AUTH_LOGGING_IN_BASE = "Logging you in";
const DOT_ANIMATION_INTERVAL_MS = 600;

const workingPlaceholderAnimation = { timer: null, phase: 0 };
const loggingInAnimation = { timer: null, phase: 0 };

/** Once coaching is entered before results, keep the banner until page reload. */
let earlyCoachingBannerLatched = false;

const INSECURE_CONTEXT_MESSAGE =
  "Authentication requires HTTPS or localhost. Browsers block encryption on plain HTTP to remote hosts. Use https://, open http://127.0.0.1/ on the server, or use an SSH tunnel.";

function hasSecureCrypto() {
  return window.isSecureContext && window.crypto && window.crypto.subtle;
}

function normalizePublicKey(pem) {
  return pem.replace(/\r/g, "").trim();
}

function isWaitingForResponse() {
  return chatInFlight || activeRecoveryPromise !== null;
}

function setChatEnabled(enabled) {
  isAuthenticated = enabled;
  appEl.classList.toggle("app-locked", !enabled);
  if (!enabled) {
    stopWorkingPlaceholder();
  }
  updateComposerEnabled();
}

function focusPrompt() {
  if (!isAuthenticated || promptEl.disabled) {
    return;
  }
  promptEl.focus({ preventScroll: true });
}

function focusPublicKeyInput() {
  if (publicKeyInputEl.disabled || authFormEl.classList.contains("hidden")) {
    return;
  }
  publicKeyInputEl.focus({ preventScroll: true });
}

function updateComposerEnabled() {
  const busy = isAuthenticated && isWaitingForResponse();
  const wasDisabled = promptEl.disabled;
  promptEl.disabled = !isAuthenticated || busy;
  sendBtn.disabled = !isAuthenticated || busy;
  chatForm.setAttribute("aria-busy", busy ? "true" : "false");
  // Disabling the textarea or the Send button drops focus to <body>, so put the
  // caret back in the composer the moment it becomes usable again — on first
  // unlock after login and after every turn finishes.
  if (wasDisabled && !promptEl.disabled) {
    focusPrompt();
  }
}

function dotAnimationText(base, phase) {
  return base + (phase === 0 ? "" : ".".repeat(phase));
}

function startDotTextAnimation(animation, baseText, setText) {
  stopDotTextAnimation(animation);
  animation.phase = 0;
  setText(dotAnimationText(baseText, 0));
  animation.timer = setInterval(() => {
    animation.phase = (animation.phase + 1) % 4;
    setText(dotAnimationText(baseText, animation.phase));
  }, DOT_ANIMATION_INTERVAL_MS);
}

function stopDotTextAnimation(animation, reset) {
  if (animation.timer !== null) {
    clearInterval(animation.timer);
    animation.timer = null;
  }
  animation.phase = 0;
  if (reset) {
    reset();
  }
}

function setPromptPlaceholderIdle() {
  promptEl.placeholder = PROMPT_PLACEHOLDER_IDLE;
}

function renderWorkingPlaceholder(text) {
  promptEl.placeholder = text;
}

function startWorkingPlaceholder() {
  stopWorkingPlaceholder();
  startDotTextAnimation(
    workingPlaceholderAnimation,
    PROMPT_PLACEHOLDER_WORKING_BASE,
    renderWorkingPlaceholder
  );
}

function stopWorkingPlaceholder() {
  stopDotTextAnimation(workingPlaceholderAnimation, () => {
    if (isAuthenticated) {
      setPromptPlaceholderIdle();
    }
  });
}

function startLoggingInAnimation() {
  stopLoggingInAnimation();
  startDotTextAnimation(loggingInAnimation, AUTH_LOGGING_IN_BASE, (text) => {
    authLoggingInTitleEl.textContent = text;
  });
}

function stopLoggingInAnimation() {
  stopDotTextAnimation(loggingInAnimation, () => {
    authLoggingInTitleEl.textContent = AUTH_LOGGING_IN_BASE;
  });
}

function assistantHasContent() {
  const last = conversation[conversation.length - 1];
  return last?.role === "assistant" && last.content.trim().length > 0;
}

let activeRecoveryTurnId = null;
let activeRecoveryPromise = null;

function scheduleChatTurnRecovery(turnId) {
  if (activeRecoveryTurnId === turnId && activeRecoveryPromise) {
    return activeRecoveryPromise;
  }

  activeRecoveryTurnId = turnId;
  updateComposerEnabled();
  activeRecoveryPromise = recoverChatTurn(turnId)
    .catch((error) => {
      if (error?.message === "Authentication required.") {
        throw error;
      }
    })
    .finally(() => {
      stopWorkingPlaceholder();
      if (activeRecoveryTurnId === turnId) {
        activeRecoveryTurnId = null;
        activeRecoveryPromise = null;
      }
      updateComposerEnabled();
    });
  return activeRecoveryPromise;
}

async function recoverChatTurn(turnId) {
  let lastError;

  for (let attempt = 0; attempt <= MAX_CHAT_RETRIES; attempt++) {
    try {
      await executeChatRequest({
        turnId,
        resetAssistant: true,
        silentReplay: true,
      });
      refreshSessionState().catch(() => {});
      return;
    } catch (error) {
      lastError = error;
      if (error.message === "Authentication required.") {
        throw error;
      }
      if (assistantHasContent()) {
        refreshSessionState().catch(() => {});
        return;
      }
      if (!isRetryableChatError(error) || attempt === MAX_CHAT_RETRIES) {
        break;
      }
      await sleep(CHAT_RETRY_DELAY_MS * (attempt + 1));
    }
  }

  for (let recovery = 0; recovery <= MAX_CHAT_RECOVERY_ATTEMPTS; recovery++) {
    try {
      await executeChatRequest({
        turnId,
        resetAssistant: true,
        silentReplay: true,
      });
      refreshSessionState().catch(() => {});
      return;
    } catch (error) {
      lastError = error;
      if (error.message === "Authentication required.") {
        throw error;
      }
      if (assistantHasContent()) {
        refreshSessionState().catch(() => {});
        return;
      }
      if (!isRetryableChatError(error) || recovery === MAX_STREAM_RECOVERY_ATTEMPTS) {
        break;
      }
      await sleep(1500 * (recovery + 1));
    }
  }

  const last = conversation[conversation.length - 1];
  if (last?.role === "assistant" && !last.content.trim()) {
    conversation.pop();
    renderMessages();
  }
  if (!assistantHasContent()) {
    showError(formatUserFacingError(lastError || new Error("Something went wrong.")));
  }
}

function getStoredPublicKey() {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) {
    return null;
  }
  const normalized = normalizePublicKey(raw);
  if (!normalized.includes("BEGIN PUBLIC KEY")) {
    return null;
  }
  return normalized;
}

function showAuthLoggingIn() {
  authGateEl.classList.remove("hidden");
  authGateEl.setAttribute("aria-hidden", "false");
  authLoggingInEl.classList.remove("hidden");
  authFormEl.classList.add("hidden");
  authErrorEl.textContent = "";
  authErrorEl.classList.add("hidden");
  setChatEnabled(false);
  renderSessionChrome(null);
  startLoggingInAnimation();
}

function showAuthLoginForm(message = "") {
  authGateEl.classList.remove("hidden");
  authGateEl.setAttribute("aria-hidden", "false");
  authLoggingInEl.classList.add("hidden");
  stopLoggingInAnimation();
  authFormEl.classList.remove("hidden");
  if (message) {
    authErrorEl.textContent = message;
    authErrorEl.classList.remove("hidden");
  } else {
    authErrorEl.textContent = "";
    authErrorEl.classList.add("hidden");
  }
  setChatEnabled(false);
  renderSessionChrome(null);
  // The gate is the first thing a user meets, so put the caret in the key box
  // straight away — on first load and whenever the form comes back after a
  // failed or expired login.
  focusPublicKeyInput();
}

function hideAuthGate() {
  stopLoggingInAnimation();
  authGateEl.classList.add("hidden");
  authGateEl.setAttribute("aria-hidden", "true");
  authErrorEl.textContent = "";
  authErrorEl.classList.add("hidden");
  setChatEnabled(true);
}

function setAuthStatus(message) {
  if (message) {
    authErrorEl.textContent = message;
    authErrorEl.classList.remove("hidden");
  } else {
    authErrorEl.textContent = "";
    authErrorEl.classList.add("hidden");
  }
}

function pemToArrayBuffer(pem) {
  const base64 = pem
    .replace(/-----BEGIN PUBLIC KEY-----/g, "")
    .replace(/-----END PUBLIC KEY-----/g, "")
    .replace(/\s/g, "");
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

function bufferToBase64(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

async function importPublicKey(pem) {
  if (!hasSecureCrypto()) {
    throw new Error(INSECURE_CONTEXT_MESSAGE);
  }

  return crypto.subtle.importKey(
    "spki",
    pemToArrayBuffer(normalizePublicKey(pem)),
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["encrypt"]
  );
}

async function encryptChallenge(publicKeyPem, nonceBase64) {
  const key = await importPublicKey(publicKeyPem);
  const nonceBytes = Uint8Array.from(atob(nonceBase64), (char) => char.charCodeAt(0));
  const ciphertext = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, key, nonceBytes);
  return bufferToBase64(ciphertext);
}

function formatAssessmentTime(isoString) {
  if (!isoString) {
    return "";
  }
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleString();
}

function renderAssessmentTimes(state) {
  const start = state?.assessmentStartTime;
  const end = state?.assessmentEndTime;

  if (!start) {
    assessmentTimesEl.classList.add("hidden");
    assessmentStartEl.textContent = "";
    assessmentEndEl.textContent = "";
    assessmentEndEl.classList.add("hidden");
    return;
  }

  assessmentTimesEl.classList.remove("hidden");
  assessmentStartEl.textContent = `Start: ${formatAssessmentTime(start)}`;

  if (end) {
    assessmentEndEl.textContent = `End: ${formatAssessmentTime(end)}`;
    assessmentEndEl.classList.remove("hidden");
  } else {
    assessmentEndEl.textContent = "";
    assessmentEndEl.classList.add("hidden");
  }
}

function renderEarlyCoachingBanner(state) {
  if (state?.coachingEnteredBeforeResults === true) {
    earlyCoachingBannerLatched = true;
  }
  if (earlyCoachingBannerLatched) {
    earlyCoachingBannerEl.classList.remove("hidden");
  } else {
    earlyCoachingBannerEl.classList.add("hidden");
  }
}

function renderSessionChrome(state) {
  renderAssessmentTimes(state);
  renderEarlyCoachingBanner(state);
}

async function refreshSessionState() {
  if (!isAuthenticated) {
    return;
  }

  const response = await fetch("/api/session/state", {
    credentials: "include",
    headers: sessionHeaders(),
  });
  if (response.status === 401) {
    return;
  }
  if (!response.ok) {
    return;
  }

  const state = await response.json();
  renderSessionChrome(state);
}

async function authenticate(publicKeyPem) {
  const challengeResponse = await fetch("/api/auth/challenge", { credentials: "include" });
  if (!challengeResponse.ok) {
    throw new Error("Failed to get authentication challenge.");
  }

  const { challenge_id: challengeID, nonce } = await challengeResponse.json();
  const ciphertext = await encryptChallenge(publicKeyPem, nonce);

  const verifyResponse = await fetch("/api/auth/verify", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      challenge_id: challengeID,
      ciphertext,
    }),
  });

  if (!verifyResponse.ok) {
    throw new Error("Authentication failed.");
  }

  const verified = await verifyResponse.json().catch(() => null);
  sessionToken = verified?.session_token || "";
}

async function bootstrapSession() {
  const response = await fetch("/api/session/bootstrap", {
    method: "POST",
    credentials: "include",
    headers: sessionHeaders({
      "Content-Type": "application/json",
      [TURN_ID_HEADER]: BOOTSTRAP_TURN_ID,
    }),
    body: JSON.stringify({ model: CHAT_MODEL }),
  });

  if (response.status === 401) {
    throw new Error("Authentication required.");
  }

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(errorText || "Failed to initialize session.");
  }

  const data = await response.json();
  conversation.length = 0;
  conversation.push({ role: "user", content: "start", internal: true });
  conversation.push({ role: "assistant", content: data.assistant || "" });
  renderMessages();
  await refreshSessionState();
}

async function completeAuthentication(publicKeyPem) {
  await authenticate(publicKeyPem);
  setAuthStatus("Initializing session...");
  await bootstrapSession();
  localStorage.setItem(STORAGE_KEY, publicKeyPem);
  hideAuthGate();
  await refreshSessionState();
}

async function initAuth() {
  setChatEnabled(false);

  if (!hasSecureCrypto()) {
    showAuthLoginForm(INSECURE_CONTEXT_MESSAGE);
    return;
  }

  const storedKey = getStoredPublicKey();
  if (storedKey) {
    showAuthLoggingIn();
    try {
      await completeAuthentication(storedKey);
      return;
    } catch {
      localStorage.removeItem(STORAGE_KEY);
      showAuthLoginForm();
      return;
    }
  }

  showAuthLoginForm();
}

function initMarkdown() {
  if (typeof marked === "undefined" || typeof DOMPurify === "undefined") {
    return;
  }
  marked.setOptions({
    gfm: true,
    breaks: true,
  });
  DOMPurify.addHook("afterSanitizeAttributes", (node) => {
    if (node.tagName === "A") {
      node.setAttribute("rel", "noopener noreferrer");
      node.setAttribute("target", "_blank");
    }
  });
}

const completeIPyFencePattern = /```(?:json)?\s*_ipy(?:intervu)?\s*\n[\s\S]*?\n```/gi;

function stripTrailingPartialIPyFence(content) {
  const fenceCount = (content.match(/```/g) || []).length;
  if (fenceCount === 0 || fenceCount % 2 === 0) {
    return content;
  }
  const open = content.lastIndexOf("```");
  const after = content.slice(open + 3).trim();
  if (
    after === "" ||
    after.startsWith("json") ||
    after.startsWith("_") ||
    after.toLowerCase().startsWith("_ipy")
  ) {
    return content.slice(0, open).replace(/[ \t\r\n]+$/, "");
  }
  return content;
}

function stripClientVisibleAssistantContent(content) {
  if (!content) {
    return "";
  }
  let stripped = content.replace(completeIPyFencePattern, "");
  stripped = stripTrailingPartialIPyFence(stripped);
  return stripped.replace(/[ \t\r\n]+$/, "");
}

function renderAssistantMarkdown(content) {
  const visible = stripClientVisibleAssistantContent(content);
  if (!visible) {
    return "";
  }
  if (typeof marked === "undefined" || typeof DOMPurify === "undefined") {
    return visible;
  }
  return DOMPurify.sanitize(marked.parse(visible), { USE_PROFILES: { html: true } });
}

function renderMessages() {
  messagesEl.innerHTML = "";

  if (conversation.length === 0) {
    const empty = document.createElement("div");
    empty.className = "message empty";
    empty.textContent = "Start a conversation by sending a message.";
    messagesEl.appendChild(empty);
    return;
  }

  for (const message of conversation) {
    if (message.internal) {
      continue;
    }

    const item = document.createElement("div");
    item.className = `message ${message.role}`;

    if (message.role === "assistant") {
      const body = document.createElement("div");
      body.className = "markdown-body";
      body.innerHTML = renderAssistantMarkdown(message.content);
      item.appendChild(body);
    } else {
      item.textContent = message.content;
    }

    messagesEl.appendChild(item);
  }

  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function showError(text) {
  conversation.push({ role: "error", content: text });
  renderMessages();
}

function formatUserFacingError(error) {
  const message = error?.message || "";
  const lowered = message.toLowerCase();

  if (isRetryableNetworkError(error)) {
    return (
      "Connection lost while waiting for a response. " +
      "Your reply may have been cut off. " +
      "Please try again, re-enter your last posting."
    );
  }

  return message || "Something went wrong.";
}

function isRetryableNetworkError(error) {
  const message = error?.message || "";
  const lowered = message.toLowerCase();

  return (
    error?.name === "TimeoutError" ||
    lowered === "load failed" ||
    lowered === "failed to fetch" ||
    lowered.includes("networkerror") ||
    lowered.includes("request timed out") ||
    (error instanceof TypeError && lowered.includes("fetch"))
  );
}

function isRetryableChatError(error) {
  if (isRetryableNetworkError(error)) {
    return true;
  }

  const message = (error?.message || "").toLowerCase();
  return message.includes("upstream error") || message.includes("bad gateway");
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function chatPayloadMessages() {
  return conversation
    .filter(
      (message) =>
        message.role === "user" ||
        (message.role === "assistant" && message.content.trim() !== "")
    )
    .map(({ role, content }) => ({
      role,
      content:
        role === "assistant" ? stripClientVisibleAssistantContent(content) : content,
    }));
}

async function executeChatRequest({
  turnId,
  resetAssistant,
  silentReplay = false,
}) {
  if (activeChatAbort) {
    activeChatAbort.abort();
  }

  const controller = new AbortController();
  activeChatAbort = controller;
  const timeoutId = setTimeout(() => {
    controller.abort(new DOMException("Request timed out", "TimeoutError"));
  }, CHAT_READ_TIMEOUT_MS);

  try {
    if (resetAssistant) {
      const last = conversation[conversation.length - 1];
      if (last?.role === "assistant") {
        last.content = "";
        if (!silentReplay) {
          renderMessages();
        }
      }
    }

    const payload = {
      model: CHAT_MODEL,
      messages: chatPayloadMessages(),
    };

    const response = await fetch("/api/chat", {
      method: "POST",
      credentials: "include",
      signal: controller.signal,
      headers: sessionHeaders({
        "Content-Type": "application/json",
        [TURN_ID_HEADER]: turnId,
      }),
      body: JSON.stringify(payload),
    });

    if (response.status === 401) {
      localStorage.removeItem(STORAGE_KEY);
      showAuthLoginForm("Your session expired or is invalid. Enter your public key again.");
      throw new Error("Authentication required.");
    }

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || `Request failed with status ${response.status}`);
    }

    const data = await response.json();
    const content = stripClientVisibleAssistantContent(
      data.choices?.[0]?.message?.content || ""
    );
    const last = conversation[conversation.length - 1];
    if (last?.role === "assistant") {
      last.content = content;
    } else {
      conversation.push({ role: "assistant", content });
    }
    renderMessages();
    refreshSessionState().catch(() => {});
  } finally {
    clearTimeout(timeoutId);
    activeChatAbort = null;
  }
}

async function sendMessage(text) {
  if (!isAuthenticated) {
    throw new Error("Authentication required.");
  }

  if (chatInFlight) {
    return;
  }

  chatInFlight = true;
  updateComposerEnabled();
  startWorkingPlaceholder();

  try {
    const turnId = crypto.randomUUID();
    conversation.push({ role: "user", content: text });
    conversation.push({ role: "assistant", content: "" });
    renderMessages();

    try {
      await executeChatRequest({ turnId, resetAssistant: false });
      refreshSessionState().catch(() => {});
    } catch (error) {
      if (error.message === "Authentication required.") {
        throw error;
      }
      await scheduleChatTurnRecovery(turnId);
    }
  } finally {
    chatInFlight = false;
    activeChatAbort = null;
    stopWorkingPlaceholder();
    updateComposerEnabled();
  }
}

authFormEl.addEventListener("submit", async (event) => {
  event.preventDefault();

  const publicKey = normalizePublicKey(publicKeyInputEl.value);
  if (!publicKey) {
    showAuthLoginForm("Enter a public key to continue.");
    return;
  }

  authSubmitBtn.disabled = true;

  try {
    await completeAuthentication(publicKey);
  } catch (error) {
    showAuthLoginForm(error.message || "Invalid public key or authentication failed.");
  } finally {
    authSubmitBtn.disabled = false;
  }
});

chatForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const text = promptEl.value.trim();
  if (!text || !isAuthenticated) {
    focusPrompt();
    return;
  }

  if (text.length > MAX_USER_MESSAGE_CHARS) {
    showError(
      `Your response was too long. Please limit your message to ${MAX_USER_MESSAGE_CHARS} characters or fewer.`
    );
    focusPrompt();
    return;
  }

  promptEl.value = "";

  try {
    await sendMessage(text);
  } catch (error) {
    if (isAuthenticated) {
      showError(formatUserFacingError(error));
    }
  } finally {
    focusPrompt();
  }
});

renderMessages();
initMarkdown();
initAuth();
