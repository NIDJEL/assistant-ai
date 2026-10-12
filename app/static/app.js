/* No framework, no CDN, no HTML injection from user text. */
import { VoiceInput, speakText } from "./voice.js";
const $ = (selector) => document.querySelector(selector);
const labels = { task: "Задача", meeting: "Встреча", reminder: "Напоминание" };
let session = localStorage.getItem("rhythm-session") || crypto.randomUUID();
localStorage.setItem("rhythm-session", session);
let period = "today",
  offset = 0,
  total = 0,
  agendaVersion = 0,
  busy = false;
let config = { timezone: "Europe/Moscow" };
let currentEvents = [],
  toastTimer,
  searchTimer;
const input = $("#message");

function toast(text) {
  const el = $("#toast");
  el.textContent = text;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), 6500);
}
async function api(path, options = {}) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 45000);
  try {
    const response = await fetch(path, {
      ...options,
      signal: controller.signal,
      headers: { "Content-Type": "application/json", ...options.headers },
    });
    let data;
    try {
      data = await response.json();
    } catch {
      throw new Error("Сервер вернул некорректный ответ.");
    }
    if (!response.ok)
      throw new Error(
        typeof data.detail === "string"
          ? data.detail
          : "Проверьте запрос и попробуйте снова.",
      );
    return data;
  } finally {
    clearTimeout(timeout);
  }
}
function localDate(delta = 0) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: config.timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date());
  const get = (t) => parts.find((p) => p.type === t).value;
  const d = new Date(`${get("year")}-${get("month")}-${get("day")}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + delta);
  return d.toISOString().slice(0, 10);
}
function addDays(value, days) {
  const d = new Date(value + "T12:00:00Z");
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}
function range() {
  const today = localDate();
  if (period === "today") return [today, addDays(today, 1)];
  if (period === "tomorrow") return [addDays(today, 1), addDays(today, 2)];
  if (period === "week") {
    const d = new Date(today + "T12:00:00Z");
    const start = addDays(today, -((d.getUTCDay() + 6) % 7));
    return [start, addDays(start, 7)];
  }
  return [];
}
function dateLabel(date) {
  if (!date) return "Без срока";
  return new Intl.DateTimeFormat("ru-RU", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(date + "T12:00:00Z"));
}
function showPending(pending) {
  $("#pending-bar").hidden = !pending;
}
function autoSize() {
  input.style.height = "auto";
  input.style.height = Math.min(input.scrollHeight, 110) + "px";
}
function eventCard(event, full = false) {
  const card = document.createElement("article");
  card.className = "event-card" + (event.status === "done" ? " done" : "");
  const time = document.createElement("div");
  time.className = "event-time";
  time.textContent = event.time || "—";
  const short = document.createElement("small");
  short.textContent = full
    ? dateLabel(event.date)
    : event.date
      ? new Intl.DateTimeFormat("ru-RU", {
          day: "numeric",
          month: "short",
          timeZone: "UTC",
        }).format(new Date(event.date + "T12:00:00Z"))
      : "Без срока";
  time.append(short);
  const details = document.createElement("div");
  details.className = "event-details";
  const title = document.createElement("h3");
  title.textContent = event.title;
  const meta = document.createElement("div");
  meta.className = "event-meta";
  const tag = document.createElement("span");
  tag.className = "kind-tag " + event.kind;
  tag.textContent = labels[event.kind];
  const id = document.createElement("span");
  id.textContent = "#" + event.id;
  meta.append(tag, id);
  if (event.kind === "meeting") {
    const dur = document.createElement("span");
    dur.textContent = event.duration_minutes + " мин";
    meta.append(dur);
  }
  details.append(title, meta);
  card.append(time, details);
  const check = document.createElement("button");
  check.className =
    "complete-button" + (event.status === "done" ? " checked" : "");
  check.textContent = event.status === "done" ? "✓" : "";
  check.title =
    event.status === "done" ? "Вернуть в планы" : "Отметить выполненным";
  check.setAttribute("aria-label", check.title + ": " + event.title);
  check.onclick = async () => {
    check.disabled = true;
    try {
      await api("/api/events/" + event.id, {
        method: "PATCH",
        body: JSON.stringify({
          status: event.status === "done" ? "open" : "done",
        }),
      });
      toast(
        event.status === "done"
          ? "Событие возвращено в планы"
          : "Отмечено выполненным",
      );
      await loadAgenda();
    } catch (e) {
      toast(e.message);
    } finally {
      check.disabled = false;
    }
  };
  card.append(check);
  return card;
}
async function loadAgenda(more = false) {
  const version = ++agendaVersion;
  if (!more) offset = 0;
  const q = $("#event-search").value.trim();
  const params = new URLSearchParams({ limit: 30, offset: String(offset) });
  const [start, end] = q ? [] : range();
  if (start) {
    params.set("start", start);
    params.set("end", end);
  }
  if (q) params.set("q", q);
  if ($("#kind-filter").value) params.set("kind", $("#kind-filter").value);
  if ($("#status-filter").value)
    params.set("status", $("#status-filter").value);
  try {
    const data = await api("/api/events?" + params);
    if (version !== agendaVersion) return;
    total = data.total;
    currentEvents = more ? [...currentEvents, ...data.events] : data.events;
    $("#event-count").textContent = total;
    const list = $("#agenda-list");
    if (!more) list.replaceChildren();
    data.events.forEach((e) => list.append(eventCard(e)));
    if (!currentEvents.length) {
      const p = document.createElement("p");
      p.className = "empty";
      const b = document.createElement("b");
      b.textContent = q ? "Ничего не нашлось" : "Здесь пока свободно";
      p.append(
        b,
        document.createTextNode(
          q
            ? "Попробуйте другое имя или тему."
            : "Добавьте событие голосом или сообщением.",
        ),
      );
      list.append(p);
    }
    offset = currentEvents.length;
    $("#load-more").hidden = offset >= total;
  } catch (e) {
    toast("Не удалось загрузить планы. " + e.message);
  }
}
function showResults(events) {
  const list = $("#result-list");
  list.replaceChildren();
  events.forEach((e) => list.append(eventCard(e, true)));
  $("#result-dialog").showModal();
}
function bubble(role, text, response = null) {
  $("#welcome").hidden = true;
  const row = document.createElement("div");
  row.className = "message " + role;
  const content = document.createElement("div");
  content.className = "message-content";
  const body = document.createElement("div");
  body.className = "bubble";
  body.textContent = text;
  content.append(body);
  if (response?.events?.length) {
    const button = document.createElement("button");
    button.className = "results-button";
    button.textContent = `Открыть события (${response.total ?? response.events.length}) ↗`;
    button.onclick = () => showResults(response.events);
    content.append(button);
  }
  if (response?.candidates?.length && response.action === "clarify") {
    const button = document.createElement("button");
    button.className = "results-button";
    button.textContent = "Показать записи для уточнения ↗";
    button.onclick = () => showResults(response.candidates);
    content.append(button);
  }
  const label = document.createElement("div");
  label.className = "message-label";
  label.textContent =
    role === "user"
      ? "Вы"
      : "Ритм" +
        (response?.provider === "rules_fallback" ? " · локальный режим" : "");
  content.append(label);
  if (role === "assistant") {
    const avatar = document.createElement("span");
    avatar.className = "message-avatar";
    avatar.textContent = "✳";
    row.append(avatar);
  }
  row.append(content);
  $("#conversation").append(row);
  $("#conversation").scrollTop = $("#conversation").scrollHeight;
  return row;
}
function speak(text) {
  if ($("#speak-toggle").checked)
    speakText(text, window.speechSynthesis, window.SpeechSynthesisUtterance);
}
async function send(message, requestId = crypto.randomUUID(), retry = false) {
  if (busy || !message.trim()) return;
  busy = true;
  $("#send-button").disabled = true;
  $("#mic-button").disabled = true;
  if (!retry) bubble("user", message);
  input.value = "";
  autoSize();
  const typing = document.createElement("div");
  typing.className = "typing";
  typing.textContent = "Ритм разбирает ваш запрос…";
  $("#conversation").append(typing);
  try {
    const data = await api("/api/chat", {
      method: "POST",
      body: JSON.stringify({
        message,
        session_id: session,
        request_id: requestId,
      }),
    });
    typing.remove();
    bubble("assistant", data.reply, data);
    showPending(data.pending);
    speak(data.reply);
    await loadAgenda();
  } catch (e) {
    typing.remove();
    const row = bubble(
      "assistant",
      "Не удалось получить ответ. " +
        (e.name === "AbortError"
          ? "Сервер отвечает слишком долго."
          : e.message),
    );
    row.querySelector(".bubble").classList.add("error-bubble");
    const btn = document.createElement("button");
    btn.className = "retry-button";
    btn.textContent = "Повторить запрос";
    btn.onclick = () => {
      btn.remove();
      send(message, requestId, true);
    };
    row.querySelector(".message-content").append(btn);
  } finally {
    busy = false;
    $("#send-button").disabled = false;
    $("#mic-button").disabled = false;
    input.focus();
  }
}
$("#chat-form").onsubmit = (e) => {
  e.preventDefault();
  send(input.value.trim());
};
input.addEventListener("input", autoSize);
input.onkeydown = (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    send(input.value.trim());
  }
};
document.querySelectorAll("[data-example]").forEach(
  (button) =>
    (button.onclick = () => {
      input.value = button.dataset.example;
      autoSize();
      input.focus();
    }),
);
document.querySelectorAll("[data-period]").forEach(
  (button) =>
    (button.onclick = () => {
      period = button.dataset.period;
      document
        .querySelectorAll("[data-period]")
        .forEach((b) => b.classList.toggle("selected", b === button));
      loadAgenda();
    }),
);
$("#kind-filter").onchange = () => loadAgenda();
$("#status-filter").onchange = () => loadAgenda();
$("#event-search").oninput = () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => loadAgenda(), 300);
};
$("#load-more").onclick = () => loadAgenda(true);
$("#refresh-events").onclick = () => loadAgenda();
$("#close-results").onclick = () => $("#result-dialog").close();
$("#cancel-pending").onclick = () => send("отмена");
$("#new-dialog").onclick = () => {
  if (busy) return;
  session = crypto.randomUUID();
  localStorage.setItem("rhythm-session", session);
  $("#conversation")
    .querySelectorAll(".message,.typing")
    .forEach((e) => e.remove());
  $("#welcome").hidden = false;
  showPending(false);
  toast("Новый диалог. Ваши события сохранены.");
};
$("#nav-events").onclick = () => {
  $(".agenda-panel").scrollIntoView({ behavior: "smooth" });
  $("#event-search").focus();
};
$("#nav-assistant").onclick = () => {
  $(".assistant-panel").scrollIntoView({ behavior: "smooth" });
  input.focus();
};
$("#import-demo").onclick = async () => {
  const btn = $("#import-demo");
  btn.disabled = true;
  try {
    const d = await api("/api/import-demo", { method: "POST" });
    toast(`Добавлено: ${d.imported}. Уже были загружены: ${d.skipped}.`);
    await loadAgenda();
  } catch (e) {
    toast(e.message);
  } finally {
    btn.disabled = false;
  }
};
$("#speak-toggle").checked = localStorage.getItem("rhythm-speak") === "true";
$("#speak-toggle").onchange = () => {
  localStorage.setItem("rhythm-speak", String($("#speak-toggle").checked));
  if (!$("#speak-toggle").checked && window.speechSynthesis)
    window.speechSynthesis.cancel();
};
function initVoice() {
  const errors = {
    unsupported:
      "Этот браузер не поддерживает голосовой ввод. Откройте приложение в Chrome или Edge, либо напишите запрос.",
    "not-allowed": "Разрешите доступ к микрофону в настройках браузера.",
    "audio-capture": "Микрофон не найден. Проверьте подключение.",
    network:
      "Сервис распознавания недоступен. Проверьте интернет или используйте текст.",
    "no-speech": "Речь не обнаружена. Попробуйте ещё раз.",
    "already-started": "Запись уже запускается. Попробуйте через секунду.",
  };
  const voice = new VoiceInput({
    Recognition: window.SpeechRecognition || window.webkitSpeechRecognition,
    onState: (recording) => {
      $("#mic-button").classList.toggle("recording", recording);
      $("#mic-button").setAttribute(
        "aria-label",
        recording ? "Остановить запись" : "Начать голосовой ввод",
      );
      $("#voice-hint").textContent = recording
        ? "Слушаю… Нажмите микрофон, чтобы закончить."
        : "Enter — отправить · Shift + Enter — новая строка";
    },
    onTranscript: (text) => {
      input.value = text;
      autoSize();
    },
    onFinal: (text) => send(text),
    onError: (error) =>
      toast(
        errors[error] ||
          "Не удалось распознать речь. Попробуйте текстовый ввод.",
      ),
  });
  $("#mic-button").title =
    "Голосовой ввод. Аудио может обрабатываться облачным сервисом браузера.";
  if (!voice.supported)
    $("#voice-hint").textContent = "Голосовой ввод: откройте в Chrome или Edge";
  $("#mic-button").onclick = () => {
    if (window.speechSynthesis) window.speechSynthesis.cancel();
    voice.toggle();
  };
}
let reminderTimer;
async function pollReminders() {
  if (document.visibilityState !== "visible") return;
  try {
    const d = await api("/api/reminders/due");
    for (const e of d.events) {
      const key = "rhythm-notified:" + e.id + ":" + e.updated_at;
      if (localStorage.getItem(key)) continue;
      localStorage.setItem(key, "true");
      new Notification("Ритм · Напоминание", {
        body: e.title,
        icon: "/static/favicon.svg",
      });
      toast("Напоминание: " + e.title);
      speak(e.title);
    }
  } catch {
    /* A transient error is retried on the next poll. */
  }
}
function enableReminderPolling() {
  if (reminderTimer) return;
  $("#notifications").textContent =
    "✓ Напоминания включены, пока открыта вкладка";
  reminderTimer = setInterval(pollReminders, 15000);
  pollReminders();
}
$("#notifications").onclick = async () => {
  if (!("Notification" in window)) {
    toast("Этот браузер не поддерживает уведомления.");
    return;
  }
  const permission = await Notification.requestPermission();
  if (permission === "granted") {
    localStorage.setItem("rhythm-notifications", "true");
    enableReminderPolling();
  } else
    toast("Уведомления не разрешены. Измените разрешения сайта в браузере.");
};
async function init() {
  try {
    config = await api("/health");
    $("#connection").lastChild.textContent = " Всё сохранено";
    const now = new Date(config.now);
    $("#weekday").textContent = new Intl.DateTimeFormat("ru-RU", {
      weekday: "long",
      timeZone: config.timezone,
    }).format(now);
    $("#today-date").textContent = new Intl.DateTimeFormat("ru-RU", {
      day: "numeric",
      month: "long",
      timeZone: config.timezone,
    }).format(now);
    $("#timezone").textContent = config.timezone;
    await loadAgenda();
    const h = await api("/api/history/" + session);
    h.exchanges.forEach((x) => {
      bubble("user", x.message);
      bubble("assistant", x.response.reply, x.response);
    });
    showPending(Boolean(h.state.pending));
    if (
      "Notification" in window &&
      Notification.permission === "granted" &&
      localStorage.getItem("rhythm-notifications") === "true"
    )
      enableReminderPolling();
  } catch (e) {
    $("#connection").lastChild.textContent = " Нет связи с сервером";
    toast("Не удалось подключиться. " + e.message);
  }
  initVoice();
}
init();
