// Bridge between the screen and Go. Inside Wails, window.go.main.App and
// window.runtime exist; outside (opening index.html in a plain browser to
// review the look) sample data takes over so the screen keeps working alone.
//
// Mock switches (query string, browser only):
//   ?name=Alice           start with a player name set
//   ?screen=settings      open the settings dialog (add &run=search to run the search)
//   ?search=none|one|many what Search finds (default: many)
//   ?play=error           how Play ends (default: success)
(function () {
  "use strict";

  const params = new URLSearchParams(location.search);
  const real = !!(window.go && window.go.main && window.go.main.App);

  // ---- event bus: Wails' or a local one ------------------------------------
  const listeners = {};
  const bus = {
    on(name, cb) {
      if (window.runtime && window.runtime.EventsOn) return window.runtime.EventsOn(name, cb);
      (listeners[name] = listeners[name] || []).push(cb);
      return () => { listeners[name] = (listeners[name] || []).filter((f) => f !== cb); };
    },
    emitLocal(name, data) {
      (listeners[name] || []).forEach((cb) => { try { cb(data); } catch (e) { console.error(e); } });
    },
  };

  const win = {
    minimise() { if (window.runtime && window.runtime.WindowMinimise) window.runtime.WindowMinimise(); },
    quit() { if (window.runtime && window.runtime.Quit) window.runtime.Quit(); else console.log("[mock] Quit"); },
  };

  if (real) {
    window.LauncherBridge = { real: true, api: window.go.main.App, bus, win, params };
    return;
  }

  // ---- sample mode ------------------------------------------------------------
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  const cfg = { gameDir: "", copyDir: "", playerName: params.get("name") || "", server: "" };

  let searchCancelled = false;

  const hits = {
    none: [],
    one: [{ path: "D:\\Games\\New World", supported: true }],
    many: [
      { path: "D:\\Games\\New World", supported: true },
      { path: "E:\\SteamLibrary\\steamapps\\common\\New World", supported: false },
    ],
  };

  const api = {
    async GetVersion() { return "0.1.0"; },
    async GetConfig() { return { ...cfg }; },
    async SaveConfig(c) {
      if (c.playerName && !/^[A-Za-z0-9_-]{3,32}$/.test(c.playerName)) throw new Error("Player names are 3 to 32 letters, digits, - or _.");
      Object.assign(cfg, c); return { ...cfg };
    },
    async DefaultServer() { return ""; },
    async PickGameDir() { return ""; },
    async PickCopyDir() { return ""; },
    async DefaultCopyDir() { return "C:\\Users\\Player\\AppData\\Local\\QuintessenceLauncher\\client"; },
    async SearchInstalls() {
      searchCancelled = false;
      for (let i = 0; i < 12 && !searchCancelled; i++) await sleep(150);
      if (searchCancelled) return { installs: [], complete: true, cancelled: true };
      return { installs: hits[params.get("search")] || hits.many, complete: true, cancelled: false };
    },
    async CancelSearch() { searchCancelled = true; },
    async Launch() {
      bus.emitLocal("app:busy", true);
      bus.emitLocal("play:status", "Preparing game files (first time only)…");
      await sleep(900);
      bus.emitLocal("play:status", "Starting…");
      await sleep(600);
      bus.emitLocal("app:busy", false);
      switch (params.get("play")) {
        case "error": return { ok: false, error: "This New World version isn't supported. Quintessence needs build 1.400.6031." };
        default: return { ok: true };
      }
    },
  };

  window.LauncherBridge = { real: false, api, bus, win, params };
})();
