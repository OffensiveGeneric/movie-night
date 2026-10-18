"use strict";

const state = { lib: { shows: [], movies: [] }, tab: "shows", query: "" };
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
  toast.timer = setTimeout(() => (t.hidden = true), 4000);
}

async function play(id) {
  try {
    const { playing } = await api("/api/play", { id });
    toast(`Opening ${playing} in Syncplay…`);
  } catch (err) {
    toast(`Couldn't play: ${err.message}`);
  }
}

// Placeholder art until posters are wired up: a stable colour per title.
function hue(text) {
  let h = 0;
  for (const c of text) h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}

function card(title, subtitle, onClick) {
  const el = document.createElement("button");
  el.className = "card";
  el.innerHTML = `<div class="art"></div><div class="name"></div><div class="sub"></div>`;
  el.querySelector(".art").style.setProperty("--hue", hue(title));
  el.querySelector(".art").textContent = title.slice(0, 1);
  el.querySelector(".name").textContent = title;
  el.querySelector(".sub").textContent = subtitle;
  el.addEventListener("click", onClick);
  return el;
}

function renderGrid() {
  const grid = $("#grid");
  grid.replaceChildren();
  const q = state.query.toLowerCase();
  const items = state.tab === "shows" ? state.lib.shows : state.lib.movies;
  for (const item of items.filter((i) => i.title.toLowerCase().includes(q))) {
    if (state.tab === "shows") {
      const n = item.episodes.length;
      grid.append(card(item.title, `${n} episode${n === 1 ? "" : "s"}`, () => openShow(item)));
    } else {
      grid.append(card(item.title, item.year ? String(item.year) : "", () => play(item.id)));
    }
  }
  if (!grid.children.length) grid.innerHTML = `<p class="empty">Nothing here yet.</p>`;
}

function openShow(show) {
  $("#grid").hidden = true;
  $("#show").hidden = false;
  $("#show-title").textContent = show.title;
  const seasons = $("#seasons");
  seasons.replaceChildren();
  const bySeason = Map.groupBy(show.episodes, (e) => e.season);
  for (const [season, episodes] of bySeason) {
    const h = document.createElement("h3");
    h.textContent = `Season ${season}`;
    const list = document.createElement("ol");
    list.className = "episodes";
    for (const e of episodes) {
      const li = document.createElement("li");
      li.innerHTML = `<span class="num"></span><span class="title"></span><button class="play">Play</button>`;
      li.querySelector(".num").textContent = e.number;
      li.querySelector(".title").textContent = e.title || `Episode ${e.number}`;
      li.querySelector(".play").addEventListener("click", () => play(e.id));
      list.append(li);
    }
    seasons.append(h, list);
  }
  window.scrollTo(0, 0);
}

function closeShow() {
  $("#show").hidden = true;
  $("#grid").hidden = false;
}

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
  state.lib = await api("/api/rescan", {});
  renderGrid();
  toast("Library refreshed");
});

api("/api/library").then((lib) => {
  state.lib = { shows: lib.shows || [], movies: lib.movies || [] };
  renderGrid();
});
