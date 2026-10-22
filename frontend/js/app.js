// Screen logic: the player panel, Play with its single status line, and the
// settings dialog (player name, server, the New World folder with Browse and
// Search, and the folder of the game copy).
(function () {
  "use strict";
  const { api, bus, win, real, params } = window.LauncherBridge;
  const $ = (id) => document.getElementById(id);

  const state = { cfg: null, busy: false, searching: false, found: [], selected: "" };

  // ---------- helpers ----------
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const errText = (e) => { const m = (e && e.message) ? e.message : String(e); return m.charAt(0).toUpperCase() + m.slice(1); };

  let toastTimer = 0;
  function toast(msg, ok) {
    const t = $("toast");
    t.textContent = msg; t.classList.toggle("is-ok", !!ok); t.hidden = false;
    clearTimeout(toastTimer); toastTimer = setTimeout(() => (t.hidden = true), ok ? 3500 : 7000);
  }

  // The one status line under Play: progress and errors.
  function setStatus(text, isError) {
    const p = $("play-status");
    p.textContent = text || "";
    p.classList.toggle("is-error", !!isError);
  }

  // ---------- Play ----------
  const hasName = () => !!(state.cfg && state.cfg.playerName);

  function updatePlay() {
    const btn = $("btn-play");
    btn.disabled = !hasName() || state.busy;
    btn.classList.toggle("is-busy", state.busy);
    btn.querySelector(".btn-play-text").textContent = state.busy ? "Launching" : "Play";
    btn.title = hasName() ? "" : "Choose a player name to play";
  }

  function setBusy(b) {
    state.busy = !!b;
    updatePlay();
    $("btn-name").disabled = $("btn-rename").disabled = state.busy;
  }

  async function launch() {
    if (state.busy) return;
    if (!hasName()) return;
    if (!state.cfg.gameDir) {
      setStatus("Choose your New World folder in Settings first.", true);
      openSettings();
      return;
    }
    setBusy(true);
    setStatus("Checking game files…");
    try {
      const r = await api.Launch();
      if (r.ok) setStatus("Game started");
      else setStatus(r.error || "Something went wrong.", true);
    } catch (e) {
      setStatus(errText(e), true);
    } finally {
      setBusy(false);
    }
  }

  // ---------- player ----------
  function renderPlayer() {
    const name = hasName() ? state.cfg.playerName : "";
    $("account-out").hidden = !!name; $("account-in").hidden = !name;
    if (name) {
      $("account-name").textContent = name;
      $("account-name").title = name;
      $("avatar").textContent = name[0].toUpperCase();
    }
    updatePlay();
  }

  // ---------- settings ----------
  function openSettings(focus) {
    $("in-name").value = (state.cfg && state.cfg.playerName) || "";
    $("in-server").value = (state.cfg && state.cfg.server) || "";
    api.DefaultServer().then((d) => { $("in-server").placeholder = d ? "Default: " + d : "host:port"; }).catch(() => {});
    $("in-game").value = (state.cfg && state.cfg.gameDir) || "";
    $("in-copy").value = (state.cfg && state.cfg.copyDir) || "";
    api.DefaultCopyDir().then((d) => { $("in-copy").placeholder = "Default: " + d; }).catch(() => {});
    state.found = []; state.selected = "";
    renderFound(); searchNote("");
    $("modal-settings").hidden = false;
    $(typeof focus === "string" ? focus : "in-game").focus();
  }
  function closeSettings() {
    if (state.searching) api.CancelSearch();
    $("modal-settings").hidden = true;
  }

  async function saveSettings(ev) {
    ev.preventDefault();
    try {
      state.cfg = await api.SaveConfig({
        ...state.cfg,
        playerName: $("in-name").value.trim(),
        server: $("in-server").value.trim(),
        gameDir: $("in-game").value.trim(),
        copyDir: $("in-copy").value.trim(),
      });
      closeSettings();
      renderPlayer();
      setStatus("");
      toast("Settings saved.", true);
    } catch (e) { toast(errText(e)); }
  }

  async function browse() {
    try {
      const dir = await api.PickGameDir();
      if (dir) { $("in-game").value = dir; state.selected = ""; renderFound(); }
    } catch (e) { toast(errText(e)); }
  }

  async function browseCopy() {
    try {
      const dir = await api.PickCopyDir();
      if (dir) $("in-copy").value = dir;
    } catch (e) { toast(errText(e)); }
  }

  // ---------- search ----------
  // note = text under the field; busy = show the spinner and the Cancel button.
  function searchNote(text, busy) {
    $("search-status").hidden = !text;
    $("search-text").textContent = text || "";
    $("search-spinner").hidden = !busy;
    $("btn-search-cancel").hidden = !busy;
  }

  function renderFound() {
    const ul = $("candidates");
    ul.innerHTML = "";
    state.found.forEach((c) => {
      const li = document.createElement("li");
      li.className = "cand" + (c.supported ? "" : " is-dim") + (state.selected && state.selected.toLowerCase() === c.path.toLowerCase() ? " is-selected" : "");
      li.innerHTML = `<span class="cand-mark"></span><span class="cand-path" title="${esc(c.path)}">${esc(c.path)}</span><span class="cand-src ${c.supported ? "ok" : "bad"}">${c.supported ? "supported" : "wrong version"}</span>`;
      li.addEventListener("click", () => { $("in-game").value = c.path; state.selected = c.path; renderFound(); });
      ul.appendChild(li);
    });
    ul.hidden = state.found.length === 0;
  }

  async function search() {
    if (state.searching) return;
    state.searching = true; state.found = []; state.selected = "";
    $("btn-search").disabled = true; renderFound(); searchNote("Searching your drives…", true);
    try {
      const r = await api.SearchInstalls();
      if (r.cancelled) { searchNote(""); return; }
      state.found = r.installs || [];
      const ok = state.found.filter((c) => c.supported);
      if (ok.length === 1) { $("in-game").value = ok[0].path; state.selected = ok[0].path; }
      renderFound();
      let note = state.found.length === 0 ? "No New World install found. Use Browse… to pick the folder."
        : ok.length === 0 ? "Found New World, but not the version this launcher needs."
        : ok.length === 1 ? "Found it." : "Pick the install you want to use.";
      if (!r.complete) note += " The search stopped early; some folders were not checked.";
      searchNote(note);
    } catch (e) { searchNote(errText(e)); }
    finally { state.searching = false; $("btn-search").disabled = false; }
  }

  // ---------- events ----------
  bus.on("app:busy", (b) => setBusy(b));
  bus.on("play:status", (s) => { if (state.busy) setStatus(s); });

  $("btn-min").addEventListener("click", win.minimise);
  $("btn-close").addEventListener("click", win.quit);
  $("btn-settings").addEventListener("click", () => openSettings());
  $("btn-settings-cancel").addEventListener("click", closeSettings);
  $("form-settings").addEventListener("submit", saveSettings);
  $("btn-browse").addEventListener("click", browse);
  $("btn-browse-copy").addEventListener("click", browseCopy);
  $("btn-search").addEventListener("click", search);
  $("btn-search-cancel").addEventListener("click", () => api.CancelSearch());
  $("btn-name").addEventListener("click", () => openSettings("in-name"));
  $("btn-rename").addEventListener("click", () => openSettings("in-name"));
  $("btn-play").addEventListener("click", launch);
  document.addEventListener("keydown", (e) => { if (e.key === "Escape" && !$("modal-settings").hidden) closeSettings(); });
  if (real) document.addEventListener("contextmenu", (e) => e.preventDefault());

  // ---------- start ----------
  (async function init() {
    $("foot-version").textContent = "v" + (await api.GetVersion());
    state.cfg = await api.GetConfig();
    renderPlayer();

    // visual review shortcuts (outside Wails only): ?screen=settings&run=search
    if (!real && params && params.get("screen") === "settings") {
      openSettings();
      if (params.get("run") === "search") search();
    }
  })().catch((e) => { console.error(e); toast("Failed to start: " + errText(e)); });
})();
