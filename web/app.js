"use strict";

const state = {
  lib: { shows: [], movies: [], continue: [] },
  status: {},
  tab: "shows",
  query: "",
  openShow: null, // show ID while the show page is open
  invite: null,
};
const $ = (sel) => document.querySelector(sel);

async function api(path, body) {
  const opts = body === undefined ? {} : {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Movie-Night": "1" },
    body: JSON.stringify(body),
  };
  const res = await fetch(path, opts);
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return res.json();
}

function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => (t.hidden = true), 5000);
}

async function play(id) {
  try {
    const { playing } = await api("/api/play", { id });
    toast(`Opening ${playing} in Syncplay…`);
    loadLibrary();
  } catch (err) {
    toast(err.message);
  }
}

const code = (e) => `S${String(e.season).padStart(2, "0")}E${String(e.number).padStart(2, "0")}`;

function gb(bytes) {
  return bytes >= 1e9 ? `${(bytes / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(bytes / 1e6))} MB`;
}

// Artwork: a stable colour per title, with the poster laid over it once the
// image has loaded. Missing images just leave the colour.
function hue(text) {
  let h = 0;
  for (const c of text) h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}

function art(el, id, title) {
  el.style.setProperty("--hue", hue(title));
  el.dataset.initial = title.slice(0, 1);
  const img = new Image();
  img.loading = "lazy";
  img.alt = "";
  img.onload = () => el.classList.add("has-art");
  img.onerror = () => img.remove();
  img.src = `/art/${id}`;
  el.append(img);
}

function card(id, title, subtitle, onClick, badge) {
  const el = document.createElement("button");
  el.className = "card";
  el.innerHTML = `<div class="poster"></div><div class="name"></div><div class="sub"></div>`;
  art(el.querySelector(".poster"), id, title);
  el.querySelector(".name").textContent = title;
  el.querySelector(".sub").textContent = subtitle;
  if (badge) {
    const b = document.createElement("span");
    b.className = `badge ${badge}`;
    b.textContent = badge;
    el.querySelector(".poster").append(b);
  }
  el.addEventListener("click", onClick);
  return el;
}

function renderContinue() {
  const row = $("#continue");
  row.replaceChildren();
  const items = state.lib.continue || [];
  $("#continue-wrap").hidden = !items.length || state.query !== "" || state.tab !== "shows";
  for (const { show, episode } of items) {
    const el = document.createElement("button");
    el.className = "next";
    el.innerHTML = `<div class="still"></div><div class="name"></div><div class="sub"></div>`;
    art(el.querySelector(".still"), episode.id, show.title);
    el.querySelector(".name").textContent = show.title;
    el.querySelector(".sub").textContent =
      `Next: ${code(episode)}${episode.title ? " · " + episode.title : ""}` +
      (episode.status === "ready" ? "" : ` (${episode.status})`);
    el.addEventListener("click", () => {
      if (episode.status === "ready") return play(episode.id);
      openShow(show.id);
      window.scrollTo(0, 0);
    });
    row.append(el);
  }
}

function renderGrid() {
  const grid = $("#grid");
  grid.replaceChildren();
  const q = state.query.toLowerCase();
  if (state.tab === "shows") {
    for (const s of state.lib.shows.filter((i) => i.title.toLowerCase().includes(q))) {
      const ready = s.episodes.filter((e) => e.status === "ready").length;
      const n = s.episodes.length;
      const sub = ready === n ? `${n} episode${n === 1 ? "" : "s"}` : `${ready} of ${n} episodes ready`;
      grid.append(card(s.id, s.title, sub, () => { openShow(s.id); window.scrollTo(0, 0); }, ready === 0 ? "downloading" : ""));
    }
  } else {
    for (const m of state.lib.movies.filter((i) => i.title.toLowerCase().includes(q))) {
      const ready = m.status === "ready";
      grid.append(card(m.id, m.title, m.year ? String(m.year) : "",
        () => (ready ? play(m.id) : toast(`${m.title} is still ${m.status}`)), ready ? "" : m.status));
    }
  }
  if (!grid.children.length) grid.innerHTML = `<p class="empty">Nothing here yet.</p>`;
  renderContinue();
}

function openShow(id) {
  const show = state.lib.shows.find((s) => s.id === id);
  if (!show) return closeShow();
  state.openShow = id;
  $("#home").hidden = true;
  $("#show").hidden = false;
  $("#show-title").textContent = show.title;
  const poster = $("#show-poster");
  poster.replaceChildren();
  poster.className = "poster";
  art(poster, show.id, show.title);

  const seasons = $("#seasons");
  seasons.replaceChildren();
  for (const [season, episodes] of Map.groupBy(show.episodes, (e) => e.season)) {
    const h = document.createElement("h3");
    h.textContent = season === 0 ? "Extras" : `Season ${season}`;
    const list = document.createElement("ol");
    list.className = "episodes";
    for (const e of episodes) {
      const li = document.createElement("li");
      li.innerHTML = `<div class="still"></div><span class="num"></span><span class="title"></span><span class="act"></span>`;
      art(li.querySelector(".still"), e.id, show.title);
      li.querySelector(".num").textContent = e.number;
      li.querySelector(".title").textContent = e.title || `Episode ${e.number}`;
      const act = li.querySelector(".act");
      if (e.status === "ready") {
        const b = document.createElement("button");
        b.className = "primary";
        b.textContent = "Play";
        b.addEventListener("click", () => play(e.id));
        act.append(b);
      } else {
        act.innerHTML = `<span class="badge ${e.status}">${e.status}</span>`;
      }
      list.append(li);
    }
    seasons.append(h, list);
  }
}

function closeShow() {
  state.openShow = null;
  $("#show").hidden = true;
  $("#home").hidden = false;
}

function render() {
  renderGrid();
  if (state.openShow) {
    const y = window.scrollY;
    openShow(state.openShow);
    window.scrollTo(0, y);
  }
}

async function loadLibrary() {
  const lib = await api("/api/library");
  state.lib = { shows: lib.shows || [], movies: lib.movies || [], continue: lib.continue || [] };
  render();
}

async function loadStatus() {
  const s = await api("/api/status");
  state.status = s;
  $("#syncplay-warning").hidden = !(s.syncplayProblems && s.syncplayProblems.length);
  const dl = $("#downloads");
  const downloads = s.downloads || [];
  dl.hidden = !downloads.length;
  dl.textContent = downloads.map((d) => `${d.library}: ${gb(d.needBytes)} still downloading`).join(" · ");
  if (s.art && s.art.running) {
    dl.hidden = false;
    dl.textContent = [dl.textContent, `Fetching artwork ${s.art.done}/${s.art.total}`].filter(Boolean).join(" · ");
  }
}

async function loadInvite() {
  const { invite, ready } = await api("/api/invite");
  const box = $("#invite");
  if (!invite) {
    state.invite = null;
    box.hidden = true;
    return;
  }
  state.invite = invite;
  $("#invite-from").textContent = invite.from || "Someone";
  $("#invite-title").textContent = invite.title;
  const join = $("#invite-join");
  join.disabled = !ready;
  join.textContent = ready ? "Join" : "Still downloading…";
  box.hidden = false;
}

// --- wiring ---------------------------------------------------------------

document.querySelectorAll("nav button").forEach((b) =>
  b.addEventListener("click", () => {
    document.querySelectorAll("nav button").forEach((x) => x.classList.toggle("active", x === b));
    state.tab = b.dataset.tab;
    closeShow();
    renderGrid();
  })
);
$("#search").addEventListener("input", (e) => {
  state.query = e.target.value;
  closeShow();
  renderGrid();
});
$("#back").addEventListener("click", closeShow);
$("#rescan").addEventListener("click", async () => {
  const lib = await api("/api/rescan", {});
  state.lib = { shows: lib.shows || [], movies: lib.movies || [], continue: lib.continue || [] };
  render();
  toast("Library refreshed");
});
$("#restart").addEventListener("click", async () => {
  try {
    const { playing } = await api("/api/restart", {});
    toast(`Reopening ${playing}. It rejoins where the room is.`);
  } catch (err) {
    toast(err.message);
  }
});
$("#invite-join").addEventListener("click", async () => {
  const inv = state.invite;
  if (!inv) return;
  await play(inv.id);
  await api("/api/invite/dismiss", { at: inv.at });
  loadInvite();
});
$("#invite-dismiss").addEventListener("click", async () => {
  if (state.invite) await api("/api/invite/dismiss", { at: state.invite.at });
  loadInvite();
});
$("#syncplay-fix").addEventListener("click", async () => {
  try {
    await api("/api/syncplay/fix", {});
    toast("Syncplay settings fixed. Use Restart player (or reopen Syncplay) to apply them.");
    loadStatus();
  } catch (err) {
    toast(`Couldn't fix settings: ${err.message}`);
  }
});

const safe = (fn) => () => fn().catch(() => {});
safe(loadLibrary)();
safe(loadStatus)();
safe(loadInvite)();
setInterval(safe(loadInvite), 5000);
setInterval(safe(loadStatus), 15000);
setInterval(safe(loadLibrary), 60000);
