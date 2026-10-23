const STORAGE_KEY = "ipytutor_public_key";

const appEl = document.getElementById("app");
const messagesEl = document.getElementById("messages");
const chatForm = document.getElementById("chat-form");
const promptEl = document.getElementById("prompt");
const sendBtn = document.getElementById("send-btn");
const actionsEl = document.getElementById("actions");
const authGateEl = document.getElementById("auth-gate");
const authFormEl = document.getElementById("auth-form");
const authLoggingInEl = document.getElementById("auth-logging-in");
const authLoggingInTitleEl = document.getElementById("auth-logging-in-title");
const publicKeyInputEl = document.getElementById("public-key-input");
const authErrorEl = document.getElementById("auth-error");
const authSubmitBtn = document.getElementById("auth-submit-btn");
const sidebarEl = document.getElementById("sidebar");
const toggleSidebarBtn = document.getElementById("toggle-sidebar");
const weekSelectEl = document.getElementById("week-select");
const lessonsBlockEl = document.getElementById("lessons-block");
const lessonListEl = document.getElementById("lesson-list");
const majorLineEl = document.getElementById("major-line");

const MAX_USER_MESSAGE_CHARS = 5000;
const MAX_CHAT_ATTEMPTS = 6;
const CHAT_RETRY_DELAY_MS = 1500;
const CHAT_READ_TIMEOUT_MS = 180_000;
const TURN_ID_HEADER = "X-Turn-Id";
const SESSION_TOKEN_HEADER = "X-Session-Token";
const PROMPT_PLACEHOLDER_IDLE = "Ask a question or type your answer…";
const PROMPT_PLACEHOLDER_WORKING_BASE = "The tutor is thinking";
const AUTH_LOGGING_IN_BASE = "Logging you in";
const DOT_ANIMATION_INTERVAL_MS = 600;

const INSECURE_CONTEXT_MESSAGE =
  "Authentication requires HTTPS or localhost. Browsers block encryption on plain HTTP to remote hosts. Use https://, open http://127.0.0.1/ on the server, or use an SSH tunnel.";

const conversation = [];
let tutorState = null;
let isAuthenticated = false;
let chatInFlight = false;
// Session token for this page only. The session cookie is shared by every tab,
// so another tab logging in would otherwise switch this page to its session.
let sessionToken = "";

const workingPlaceholderAnimation = { timer: null, phase: 0 };
const loggingInAnimation = { timer: null, phase: 0 };

function sessionHeaders(extra = {}) {
  return sessionToken ? { ...extra, [SESSION_TOKEN_HEADER]: sessionToken } : extra;
}

function storageGet(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function storageSet(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Private windows may refuse storage; the page still works without it.
  }
}

function storageRemove(key) {
  try {
    localStorage.removeItem(key);
  } catch {
    // ignore
  }
}

function hasSecureCrypto() {
  return window.isSecureContext && window.crypto && window.crypto.subtle;
}

function normalizePublicKey(pem) {
  return pem.replace(/\r/g, "").trim();
}

// ---------------------------------------------------------------- composer

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

function updateComposerEnabled() {
  const busy = isAuthenticated && chatInFlight;
  const wasDisabled = promptEl.disabled;
  promptEl.disabled = !isAuthenticated || busy;
  sendBtn.disabled = !isAuthenticated || busy;
  weekSelectEl.disabled = !isAuthenticated || busy;
  for (const button of document.querySelectorAll("#actions button, #lesson-list button")) {
    button.disabled = !isAuthenticated || busy;
  }
  chatForm.setAttribute("aria-busy", busy ? "true" : "false");
  // Disabling the textarea drops focus to <body>, so put the caret back in the
  // composer the moment it becomes usable again.
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

function startWorkingPlaceholder() {
  startDotTextAnimation(workingPlaceholderAnimation, PROMPT_PLACEHOLDER_WORKING_BASE, (text) => {
    promptEl.placeholder = text;
  });
}

function stopWorkingPlaceholder() {
  stopDotTextAnimation(workingPlaceholderAnimation, () => {
    promptEl.placeholder = PROMPT_PLACEHOLDER_IDLE;
  });
}

// ---------------------------------------------------------------- auth

function getStoredPublicKey() {
  const raw = storageGet(STORAGE_KEY);
  if (!raw) {
    return null;
  }
  const normalized = normalizePublicKey(raw);
  return normalized.includes("BEGIN PUBLIC KEY") ? normalized : null;
}

function showAuthLoggingIn() {
  authGateEl.classList.remove("hidden");
  authGateEl.setAttribute("aria-hidden", "false");
  authLoggingInEl.classList.remove("hidden");
  authFormEl.classList.add("hidden");
  setAuthStatus("");
  setChatEnabled(false);
  startDotTextAnimation(loggingInAnimation, AUTH_LOGGING_IN_BASE, (text) => {
    authLoggingInTitleEl.textContent = text;
  });
}

function showAuthLoginForm(message = "") {
  authGateEl.classList.remove("hidden");
  authGateEl.setAttribute("aria-hidden", "false");
  authLoggingInEl.classList.add("hidden");
  stopDotTextAnimation(loggingInAnimation);
  authFormEl.classList.remove("hidden");
  setAuthStatus(message);
  setChatEnabled(false);
  publicKeyInputEl.focus({ preventScroll: true });
}

function hideAuthGate() {
  stopDotTextAnimation(loggingInAnimation);
  authGateEl.classList.add("hidden");
  authGateEl.setAttribute("aria-hidden", "true");
  setAuthStatus("");
  setChatEnabled(true);
}

function setAuthStatus(message) {
  authErrorEl.textContent = message || "";
  authErrorEl.classList.toggle("hidden", !message);
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
  let binary = "";
  for (const byte of new Uint8Array(buffer)) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

async function encryptChallenge(publicKeyPem, nonceBase64) {
  if (!hasSecureCrypto()) {
    throw new Error(INSECURE_CONTEXT_MESSAGE);
  }
  const key = await crypto.subtle.importKey(
    "spki",
    pemToArrayBuffer(normalizePublicKey(publicKeyPem)),
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["encrypt"]
  );
  const nonceBytes = Uint8Array.from(atob(nonceBase64), (char) => char.charCodeAt(0));
  const ciphertext = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, key, nonceBytes);
  return bufferToBase64(ciphertext);
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
    body: JSON.stringify({ challenge_id: challengeID, ciphertext }),
  });
  if (!verifyResponse.ok) {
    throw new Error("Authentication failed.");
  }
  const verified = await verifyResponse.json().catch(() => null);
  sessionToken = verified?.session_token || "";
}

async function completeAuthentication(publicKeyPem) {
  await authenticate(publicKeyPem);
  setAuthStatus("Getting your tutor ready...");
  await bootstrapSession();
  storageSet(STORAGE_KEY, publicKeyPem);
  hideAuthGate();
}

async function initAuth() {
  setChatEnabled(false);
  if (!hasSecureCrypto()) {
    showAuthLoginForm(INSECURE_CONTEXT_MESSAGE);
    return;
  }
  const storedKey = getStoredPublicKey();
  if (!storedKey) {
    showAuthLoginForm();
    return;
  }
  showAuthLoggingIn();
  try {
    await completeAuthentication(storedKey);
  } catch {
    storageRemove(STORAGE_KEY);
    showAuthLoginForm();
  }
}

function handleUnauthorized() {
  storageRemove(STORAGE_KEY);
  showAuthLoginForm("Your session expired or is invalid. Enter your public key again.");
  return new Error("Authentication required.");
}

// ---------------------------------------------------------------- tutor state

async function bootstrapSession() {
  const response = await fetch("/api/session/bootstrap", {
    method: "POST",
    credentials: "include",
    headers: sessionHeaders({ "Content-Type": "application/json" }),
    body: "{}",
  });
  if (response.status === 401) {
    throw new Error("Authentication required.");
  }
  if (!response.ok) {
    throw new Error((await response.text()) || "Failed to start the tutor.");
  }
  const data = await response.json();
  conversation.length = 0;
  conversation.push({ role: "user", content: "start", internal: true });
  conversation.push({ role: "assistant", content: data.assistant || "" });
  renderMessages();
  applyState(data.state);
}

function applyState(state) {
  if (!state) {
    return;
  }
  tutorState = state;
  renderSidebar();
  renderActions();
}

function renderSidebar() {
  const state = tutorState || {};
  const week = state.week;

  // Keep the options; only rebuild them when the week list changes (it doesn't, in practice).
  if (weekSelectEl.options.length !== (state.weeks || []).length + 1) {
    weekSelectEl.length = 1;
    for (const option of state.weeks || []) {
      const el = document.createElement("option");
      el.value = String(option.number);
      el.textContent = option.title;
      weekSelectEl.appendChild(el);
    }
  }
  weekSelectEl.value = week ? String(week.number) : "";

  lessonsBlockEl.classList.toggle("hidden", !week);
  lessonListEl.innerHTML = "";
  const current = state.activity?.kind === "lesson" ? state.activity.lesson : 0;
  for (const [index, title] of (week?.lessons || []).entries()) {
    const number = index + 1;
    const item = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.className = number === current ? "lesson-button current" : "lesson-button";
    button.textContent = `${number}. ${title}`;
    if (number === current) {
      button.setAttribute("aria-current", "true");
    }
    button.disabled = !isAuthenticated || chatInFlight;
    button.addEventListener("click", () => {
      runUserTurn(`Lesson ${number}: ${title}`, `lesson:${number}`);
      closeSidebarOnMobile();
    });
    item.appendChild(button);
    lessonListEl.appendChild(item);
  }

  majorLineEl.classList.toggle("hidden", !state.major);
  majorLineEl.textContent = state.major ? `Examples set in: ${state.major}` : "";
}

// renderActions offers buttons for what the student can do next.
function renderActions() {
  actionsEl.innerHTML = "";
  const state = tutorState;
  const buttons = [];
  if (state?.phase === "welcome" || state?.phase === "choose_week") {
    for (const option of state.weeks || []) {
      buttons.push({ label: `Week ${option.number}`, action: `week:${option.number}`, text: `Week ${option.number}` });
    }
  } else if (state?.phase === "studying" && state.week) {
    const activity = state.activity;
    const practicing = activity && activity.kind !== "lesson";
    if (practicing && activity.open) {
      buttons.push({ label: "💡 Hint", action: "hint", primary: true, kind: "hint" });
      buttons.push({ label: "✅ Show solution", action: "solution", kind: activity.kind });
    } else if (practicing) {
      buttons.push({ label: "🔁 Another one", action: "another", primary: true, kind: activity.kind });
      buttons.push({ label: "⬆️ Harder one", action: "another:harder", text: "A harder one", kind: activity.kind });
    }
    const lessonCount = state.week.lessons?.length || 0;
    if (activity?.kind === "lesson") {
      if (activity.lesson < lessonCount) {
        buttons.push({ label: "➡️ Next lesson", action: "next_lesson", primary: true, kind: "lesson" });
      }
    } else {
      buttons.push({ label: "📖 Teach me", action: "lesson", primary: !activity, kind: "lesson" });
    }
    for (const kind of state.week.practiceKinds || []) {
      const emoji = { concept: "💬", code: "🧩", bug: "🐞" }[kind.kind] || "📝";
      buttons.push({ label: `${emoji} ${kind.label}`, action: `practice:${kind.kind}`, kind: kind.kind });
    }
    buttons.push({ label: "🎯 Interview guide", action: "guide" });
  }
  for (const spec of buttons) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = spec.primary ? "chip primary" : "chip";
    if (spec.kind) {
      button.dataset.kind = spec.kind;
    }
    button.textContent = spec.label;
    button.disabled = !isAuthenticated || chatInFlight;
    button.addEventListener("click", () => {
      runUserTurn(spec.text || spec.label, spec.action);
      closeSidebarOnMobile();
    });
    actionsEl.appendChild(button);
  }
  actionsEl.classList.toggle("hidden", buttons.length === 0);
}

// ---------------------------------------------------------------- messages

function initMarkdown() {
  if (typeof marked === "undefined" || typeof DOMPurify === "undefined") {
    return;
  }
  marked.setOptions({ gfm: true, breaks: true });
  DOMPurify.addHook("afterSanitizeAttributes", (node) => {
    if (node.tagName === "A") {
      node.setAttribute("rel", "noopener noreferrer");
      node.setAttribute("target", "_blank");
    }
  });
}

function renderMarkdown(content) {
  if (typeof marked === "undefined" || typeof DOMPurify === "undefined") {
    const div = document.createElement("div");
    div.textContent = content;
    return div.innerHTML;
  }
  return DOMPurify.sanitize(marked.parse(content), { USE_PROFILES: { html: true } });
}

// messageKind names the activity a reply belongs to, from how the server starts it, so it
// can carry the same color as the button that asked for it.
function messageKind(content) {
  const text = content.replace(/^\*📚[^\n]*\n+/, "");
  if (/^### 💬/.test(text)) return "concept";
  if (/^### 🧩/.test(text)) return "code";
  if (/^### 🐞/.test(text)) return "bug";
  if (text.startsWith("💡")) return "hint";
  if (/\n\*Lesson \d+ of \d+/.test(text)) return "lesson";
  return "";
}

// separateCallouts puts each feedback callout line in its own paragraph; the model often
// writes "Interview check" and "Interview prep" on consecutive lines.
function separateCallouts(content) {
  return content.replace(/\n(?=[ \t]*\*\*Interview (?:check|prep):\*\*)/g, "\n\n");
}

const CALLOUTS = { "Interview check:": "callout-check", "Interview prep:": "callout-prep" };
const RATINGS = { Exceptional: "rating-exceptional", Competent: "rating-competent", "Not Ready Yet": "rating-not-ready" };

// decorateFeedback styles the "Interview check" and "Interview prep" paragraphs as
// callouts, and colors the rating word in the check.
function decorateFeedback(container) {
  for (const p of container.querySelectorAll("p")) {
    const lead = p.firstElementChild;
    if (!lead || lead.tagName !== "STRONG" || !p.textContent.trimStart().startsWith(lead.textContent)) {
      continue;
    }
    const callout = CALLOUTS[lead.textContent.trim()];
    if (!callout) {
      continue;
    }
    p.classList.add("callout", callout);
    if (callout === "callout-check") {
      for (const strong of p.querySelectorAll("strong")) {
        const rating = RATINGS[strong.textContent.trim().replace(/[.:,]$/, "")];
        if (rating) {
          strong.classList.add(rating);
        }
      }
    }
  }
}

function addCopyButtons(container) {
  for (const pre of container.querySelectorAll("pre")) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "copy-code ghost small";
    button.textContent = "Copy";
    button.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText((pre.querySelector("code") || pre).innerText);
        button.textContent = "Copied";
        setTimeout(() => (button.textContent = "Copy"), 1500);
      } catch {
        button.textContent = "Select and copy";
      }
    });
    pre.appendChild(button);
  }
}

function renderMessages() {
  messagesEl.innerHTML = "";
  for (const message of conversation) {
    if (message.internal) {
      continue;
    }
    const item = document.createElement("div");
    item.className = `message ${message.role}`;
    if (message.role === "assistant") {
      if (!message.content) {
        item.classList.add("pending");
        item.innerHTML = '<span class="typing" aria-label="The tutor is thinking"><span></span><span></span><span></span></span>';
      } else {
        const body = document.createElement("div");
        body.className = "markdown-body";
        body.innerHTML = renderMarkdown(separateCallouts(message.content));
        decorateFeedback(body);
        addCopyButtons(body);
        const kind = messageKind(message.content);
        if (kind) {
          item.dataset.kind = kind;
        }
        item.appendChild(body);
      }
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

// ---------------------------------------------------------------- chat turns

function isRetryableNetworkError(error) {
  const lowered = (error?.message || "").toLowerCase();
  return (
    error?.name === "TimeoutError" ||
    lowered === "load failed" ||
    lowered === "failed to fetch" ||
    lowered.includes("networkerror") ||
    lowered.includes("request timed out") ||
    lowered.includes("upstream error") ||
    lowered.includes("bad gateway") ||
    (error instanceof TypeError && lowered.includes("fetch"))
  );
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function chatPayloadMessages() {
  return conversation
    .filter((m) => m.role === "user" || (m.role === "assistant" && m.content.trim() !== ""))
    .map(({ role, content }) => ({ role, content }));
}

// postChatTurn sends one turn. Retries reuse the turn id, so the server replays the reply
// it already produced instead of running the turn twice.
async function postChatTurn(turnId, action) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => {
    controller.abort(new DOMException("Request timed out", "TimeoutError"));
  }, CHAT_READ_TIMEOUT_MS);
  try {
    const response = await fetch("/api/chat", {
      method: "POST",
      credentials: "include",
      signal: controller.signal,
      headers: sessionHeaders({ "Content-Type": "application/json", [TURN_ID_HEADER]: turnId }),
      body: JSON.stringify({
        messages: chatPayloadMessages(),
        action: action || "",
      }),
    });
    if (response.status === 401) {
      throw handleUnauthorized();
    }
    if (response.status === 409) {
      throw new Error("upstream error: turn still running");
    }
    if (!response.ok) {
      throw new Error((await response.text()) || `Request failed with status ${response.status}`);
    }
    return await response.json();
  } finally {
    clearTimeout(timeoutId);
  }
}

async function runUserTurn(text, action = "") {
  if (!isAuthenticated || chatInFlight) {
    return;
  }
  chatInFlight = true;
  updateComposerEnabled();
  startWorkingPlaceholder();
  conversation.push({ role: "user", content: text });
  const reply = { role: "assistant", content: "" };
  conversation.push(reply);
  renderMessages();

  const turnId = crypto.randomUUID();
  try {
    let lastError = null;
    for (let attempt = 0; attempt < MAX_CHAT_ATTEMPTS; attempt += 1) {
      try {
        const data = await postChatTurn(turnId, action);
        reply.content = data.choices?.[0]?.message?.content || "";
        renderMessages();
        applyState(data.state);
        lastError = null;
        break;
      } catch (error) {
        lastError = error;
        if (error.message === "Authentication required." || !isRetryableNetworkError(error)) {
          break;
        }
        await sleep(CHAT_RETRY_DELAY_MS * (attempt + 1));
      }
    }
    if (lastError) {
      conversation.splice(conversation.indexOf(reply), 1);
      if (lastError.message !== "Authentication required.") {
        showError(
          isRetryableNetworkError(lastError)
            ? "Connection lost while waiting for the tutor. Please send your message again."
            : lastError.message || "Something went wrong."
        );
      } else {
        renderMessages();
      }
    }
  } finally {
    chatInFlight = false;
    stopWorkingPlaceholder();
    updateComposerEnabled();
    renderActions();
  }
}

// ---------------------------------------------------------------- events

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

chatForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const text = promptEl.value.trim();
  if (!text || !isAuthenticated) {
    focusPrompt();
    return;
  }
  if (text.length > MAX_USER_MESSAGE_CHARS) {
    showError(`Your message is too long. Please keep it to ${MAX_USER_MESSAGE_CHARS} characters or fewer.`);
    return;
  }
  promptEl.value = "";
  runUserTurn(text);
});

promptEl.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
    event.preventDefault();
    chatForm.requestSubmit();
  }
});

weekSelectEl.addEventListener("change", () => {
  const number = Number(weekSelectEl.value);
  if (number && number !== tutorState?.week?.number) {
    runUserTurn(`Study Week ${number}`, `week:${number}`);
    closeSidebarOnMobile();
  }
});

function closeSidebarOnMobile() {
  sidebarEl.classList.remove("open");
  toggleSidebarBtn.setAttribute("aria-expanded", "false");
}

toggleSidebarBtn.addEventListener("click", () => {
  const open = sidebarEl.classList.toggle("open");
  toggleSidebarBtn.setAttribute("aria-expanded", open ? "true" : "false");
});

initMarkdown();
renderMessages();
renderActions();
initAuth();
