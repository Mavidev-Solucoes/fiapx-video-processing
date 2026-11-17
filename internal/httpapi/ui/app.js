"use strict";

let accessToken = "";
let activeEmail = "";
let refreshing = false;
const el = id => document.getElementById(id);

function message(text, bad = false) {
  el("message").textContent = text;
  el("message").classList.toggle("problem", bad);
}

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (accessToken) headers.set("Authorization", "Bearer " + accessToken);
  const response = await fetch(path, { ...options, headers });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || "Erro HTTP " + response.status);
  }
  if (response.status === 204) return null;
  if (options.download) return response.blob();
  return response.json();
}

function make(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (className) node.className = className;
  return node;
}

function showSession() {
  el("authPanel").hidden = Boolean(accessToken);
  el("dashboard").hidden = !accessToken;
  el("userBar").hidden = !accessToken;
  el("userEmail").textContent = activeEmail;
}

async function authenticate(register) {
  const email = el("email").value.trim();
  const password = el("password").value;
  const route = register ? "/api/auth/register" : "/api/auth/login";
  const data = await request(route, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password })
  });
  accessToken = data.accessToken;
  activeEmail = data.user.email;
  el("password").value = "";
  showSession();
  message("Sessão iniciada com sucesso.");
  await refresh();
}

function logout() {
  accessToken = "";
  activeEmail = "";
  el("jobs").replaceChildren();
  el("notifications").replaceChildren();
  showSession();
  message("Sessão encerrada.");
}

async function downloadJob(job) {
  try {
    const blob = await request("/api/jobs/" + encodeURIComponent(job.id) + "/download", { download: true });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "fiapx-" + job.id + ".zip";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    message("Download iniciado.");
  } catch (error) {
    message(error.message, true);
  }
}

function renderJobs(jobs) {
  const list = el("jobs");
  list.replaceChildren();
  if (!jobs.length) { list.append(make("li", "Nenhum vídeo enviado.")); return; }
  const labels = { queued: "Na fila", processing: "Processando", completed: "Concluído", failed: "Falhou" };
  for (const job of jobs) {
    const item = make("li");
    item.append(make("div", job.filename, "titleline"));
    const status = labels[job.status] || job.status;
    item.append(make("div", status + " · " + new Date(job.createdAt).toLocaleString("pt-BR"),
      "meta " + (job.status === "failed" ? "error" : job.status === "completed" ? "ok" : "wait")));
    if (job.errorMessage) item.append(make("div", job.errorMessage, "error"));
    if (job.status === "completed") {
      const button = make("button", "Baixar ZIP");
      button.addEventListener("click", () => downloadJob(job));
      item.append(button);
    }
    list.append(item);
  }
}

async function acknowledge(id) {
  try {
    await request("/api/notifications/" + encodeURIComponent(id) + "/read", { method: "POST" });
    await refresh();
  } catch (error) { message(error.message, true); }
}

function renderNotifications(notifications) {
  const list = el("notifications");
  list.replaceChildren();
  let unread = 0;
  if (!notifications.length) { list.append(make("li", "Nenhuma notificação.")); }
  for (const note of notifications) {
    const item = make("li", undefined, note.readAt ? "" : "unread");
    if (!note.readAt) unread++;
    item.append(make("div", note.message, "titleline error"));
    item.append(make("div", "Trabalho: " + note.jobId + " · " + new Date(note.createdAt).toLocaleString("pt-BR"), "meta"));
    if (!note.readAt) {
      const button = make("button", "Marcar como lida", "outline");
      button.addEventListener("click", () => acknowledge(note.id));
      item.append(button);
    }
    list.append(item);
  }
  el("unreadCount").textContent = unread ? "(" + unread + " não lidas)" : "";
}

async function refresh() {
  if (!accessToken || refreshing) return;
  refreshing = true;
  try {
    const [jobs, notifications] = await Promise.all([
      request("/api/jobs"), request("/api/notifications")
    ]);
    renderJobs(jobs.jobs || []);
    renderNotifications(notifications.notifications || []);
  } catch (error) {
    if (error.message.includes("token")) logout();
    message("Não foi possível atualizar: " + error.message, true);
  } finally { refreshing = false; }
}

el("authForm").addEventListener("submit", async event => {
  event.preventDefault();
  try { await authenticate(false); } catch (error) { message(error.message, true); }
});
el("register").addEventListener("click", async () => {
  try { await authenticate(true); } catch (error) { message(error.message, true); }
});
el("uploadForm").addEventListener("submit", async event => {
  event.preventDefault();
  const file = el("video").files[0];
  if (!file) return;
  if (file.size > 100 * 1024 * 1024) { message("O limite é 100 MiB.", true); return; }
  const submit = el("upload");
  submit.disabled = true;
  try {
    const form = new FormData();
    form.append("video", file);
    await request("/api/jobs", { method: "POST", body: form });
    el("video").value = "";
    message("Vídeo recebido. O processamento começou na fila.");
    await refresh();
  } catch (error) { message(error.message, true); }
  finally { submit.disabled = false; }
});
el("refresh").addEventListener("click", refresh);
el("logout").addEventListener("click", logout);
showSession();
setInterval(refresh, 4000);
