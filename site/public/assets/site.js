// confdrift site: copy buttons, install tabs and the playground. The
// playground runs the real engine (Go compiled to WebAssembly), loaded the
// first time the playground comes near the screen.

const T = {
  en: {
    copy: "copy", copied: "copied",
    loading: "Loading the engine…",
    failed: "The engine did not load. Refresh the page, or try the CLI.",
    exit: { 0: "no drift", 1: "drift found", 2: "bad input" },
  },
  "pt-BR": {
    copy: "copiar", copied: "copiado",
    loading: "Carregando o motor…",
    failed: "O motor não carregou. Recarregue a página, ou use a CLI.",
    exit: { 0: "sem drift", 1: "drift encontrado", 2: "entrada inválida" },
  },
};
const t = T[document.documentElement.lang] || T.en;

// ── Copy buttons ───────────────────────────────────────────────
document.querySelectorAll("[data-copy]").forEach((btn) => {
  btn.textContent = t.copy;
  btn.addEventListener("click", async () => {
    const src = document.getElementById(btn.dataset.copy);
    const text = src.dataset.text || src.innerText.replace(/^\$ /gm, "");
    try {
      await navigator.clipboard.writeText(text.trim());
      btn.textContent = t.copied;
      btn.classList.add("done");
      setTimeout(() => { btn.textContent = t.copy; btn.classList.remove("done"); }, 1600);
    } catch { /* clipboard blocked: the text is still selectable */ }
  });
});

// ── Tabs (install, playground output) ──────────────────────────
document.querySelectorAll('[role="tablist"]').forEach((list) => {
  const tabs = [...list.querySelectorAll('[role="tab"]')];
  const select = (tab) => {
    tabs.forEach((x) => {
      const on = x === tab;
      x.setAttribute("aria-selected", on);
      x.tabIndex = on ? 0 : -1;
      const panel = document.getElementById(x.getAttribute("aria-controls"));
      if (panel) panel.hidden = !on;
    });
    list.dispatchEvent(new CustomEvent("tabchange", { detail: tab }));
  };
  tabs.forEach((tab, i) => {
    tab.addEventListener("click", () => select(tab));
    tab.addEventListener("keydown", (e) => {
      const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (!d) return;
      const next = tabs[(i + d + tabs.length) % tabs.length];
      next.focus();
      select(next);
    });
  });
  // Install tabs open on the visitor's platform.
  if (list.dataset.platform !== undefined) {
    const ua = navigator.userAgent;
    const id = /Windows/.test(ua) ? "win" : /Mac/.test(ua) ? "mac" : "linux";
    const tab = tabs.find((x) => x.dataset.os === id);
    if (tab) select(tab);
  }
});

// ── Playground ─────────────────────────────────────────────────
const play = document.getElementById("playground");
if (play) setupPlayground(play);

function setupPlayground(root) {
  const PRESETS = {
    k8s: [
      ["staging/configmap.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout
spec:
  replicas: 2
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: checkout-env
data:
  LOG_LEVEL: debug
  PAYMENTS_TIMEOUT_MS: "3000"
  FEATURE_NEW_CART: "true"
  DATABASE_URL: postgres://checkout:example@db.staging.example.com:5432/checkout
  STRIPE_API_KEY: sk_test_example
  RETRY_LIMIT: "5"
`],
      ["production/configmap.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: checkout-env
data:
  LOG_LEVEL: info
  PAYMENTS_TIMEOUT_MS: "300"
  DATABASE_URL: postgres://checkout:example@db.prod.example.com:5432/checkout
  STRIPE_API_KEY: sk_live_example
  RETRY_LIMIT: "5"
`],
    ],
    dotenv: [
      [".env.staging", `# checkout service, staging
export APP_ENV=staging
LOG_LEVEL=debug
CACHE_TTL_SECONDS=60
SMTP_PASSWORD="not-a-real-password-1"
REDIS_URL=redis://:example@cache.staging.internal:6379/0
SENTRY_DSN=https://example@o1.ingest.example.io/42
`],
      [".env.production", `# checkout service, production
export APP_ENV=production
LOG_LEVEL=info
CACHE_TTL_SECONDS=60
SMTP_PASSWORD="not-a-real-password-2"
REDIS_URL=redis://:example@cache.prod.internal:6379/0
`],
    ],
    formats: [
      ["app.yaml", `server:
  port: 8080
  timeout: 30s
db:
  host: db.internal
  port: 5432
  pool:
    max: 20
features:
  - search
  - cart
`],
      ["app.json", `{
  "server": { "port": 8080, "timeout": "30s" },
  "db": {
    "host": "db.internal",
    "port": "5432",
    "pool": { "max": 10 }
  },
  "features": ["search", "cart", "wishlist"]
}
`],
    ],
    three: [
      ["dev.env", `API_BASE=http://localhost:8080
HTTP_PORT=8080
LOG_FORMAT=json
WORKERS=2
RATE_LIMIT=0
DEBUG_TOOLBAR=true
`],
      ["staging.env", `API_BASE=https://api.staging.example.com
HTTP_PORT=8080
LOG_FORMAT=json
WORKERS=4
RATE_LIMIT=100
`],
      ["prod.env", `API_BASE=https://api.example.com
HTTP_PORT=8080
LOG_FORMAT=json
WORKERS=16
RATE_LIMIT=100
TRUSTED_PROXIES=10.0.0.0/8
`],
    ],
  };

  const filesEl = root.querySelector(".files");
  const out = root.querySelector(".out");
  const exitEl = root.querySelector(".exit");
  const cmd = root.querySelector(".cmdline");
  const keysOnly = root.querySelector("#opt-keys");
  const showSecrets = root.querySelector("#opt-secrets");
  const ignore = root.querySelector("#opt-ignore");
  const addBtn = root.querySelector(".add");
  const chips = [...root.querySelectorAll("[data-preset]")];
  let view = "text";
  let engine = null;
  let last = null;

  const fmtOf = (name) => {
    const base = name.split("/").pop();
    const ext = base.includes(".") ? base.slice(base.lastIndexOf(".")).toLowerCase() : "";
    if (ext === ".json") return "json";
    if (ext === ".yaml" || ext === ".yml") return "yaml";
    if (ext === ".env" || base === ".env" || base.startsWith(".env.")) return "dotenv";
    return "?";
  };

  function fileEl(name, text) {
    const el = document.createElement("div");
    el.className = "file";
    el.innerHTML = `<div class="file-name"><input spellcheck="false" aria-label="file name"><span class="fmt"></span><button class="rm" type="button" aria-label="remove">×</button></div><textarea spellcheck="false" aria-label="file contents"></textarea>`;
    const input = el.querySelector("input");
    const area = el.querySelector("textarea");
    const fmt = el.querySelector(".fmt");
    input.value = name;
    area.value = text;
    const sync = () => { fmt.textContent = fmtOf(input.value); };
    sync();
    input.addEventListener("input", () => { sync(); edited(); });
    area.addEventListener("input", edited);
    area.addEventListener("keydown", (e) => {
      if (e.key !== "Tab" || e.shiftKey) return;
      e.preventDefault();
      area.setRangeText("  ", area.selectionStart, area.selectionEnd, "end");
      edited();
    });
    el.querySelector(".rm").addEventListener("click", () => { el.remove(); layout(); changed(); });
    return el;
  }

  function layout() {
    const n = filesEl.children.length;
    filesEl.style.setProperty("--cols", Math.min(n, 3));
    filesEl.querySelectorAll(".rm").forEach((b) => { b.hidden = n <= 2; });
    addBtn.disabled = n >= 4;
  }

  function load(preset) {
    chips.forEach((c) => c.setAttribute("aria-pressed", c.dataset.preset === preset));
    filesEl.replaceChildren(...PRESETS[preset].map(([n, s]) => fileEl(n, s)));
    ignore.value = preset === "three" ? "API_BASE" : "";
    layout();
    changed();
  }

  const quote = (s) => (/^[\w./@%+=:,-]+$/.test(s) ? s : `'${s.replace(/'/g, "'\\''")}'`);
  const patterns = () => ignore.value.split(/[\s,]+/).filter(Boolean);

  function request() {
    const files = [...filesEl.children].map((f) => ({
      name: f.querySelector("input").value.trim(),
      text: f.querySelector("textarea").value,
    }));
    return { files, keysOnly: keysOnly.checked, showSecrets: showSecrets.checked, ignore: patterns() };
  }

  function showCommand(req) {
    const parts = ["confdrift", ...req.files.map((f) => quote(f.name || "?"))];
    if (req.keysOnly) parts.push("--keys-only");
    req.ignore.forEach((p) => parts.push("--ignore", quote(p)));
    if (req.showSecrets) parts.push("--show-secrets");
    if (view === "json") parts.push("--format", "json");
    cmd.textContent = parts.join(" ");
  }

  // Once the files are edited they are no longer the preset.
  function edited() {
    chips.forEach((c) => c.setAttribute("aria-pressed", false));
    changed();
  }

  let timer;
  function changed() {
    showCommand(request());
    clearTimeout(timer);
    timer = setTimeout(runNow, 120);
  }

  function runNow() {
    if (!engine) return;
    const req = request();
    showCommand(req);
    last = JSON.parse(engine(JSON.stringify(req)));
    render();
  }

  // ANSI from the engine's text renderer to spans: the page shows exactly
  // what a terminal would.
  const SGR = { 1: "c-b", 2: "c-d", 31: "c-r", 33: "c-y" };
  const esc = (s) => s.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));
  function ansi(s) {
    let open = false;
    return esc(s).replace(/\x1b\[(\d+)m/g, (_, n) => {
      const close = open ? "</span>" : "";
      open = n !== "0" && !!SGR[n];
      return close + (open ? `<span class="${SGR[n]}">` : "");
    }) + (open ? "</span>" : "");
  }

  function render() {
    if (!last) return;
    exitEl.dataset.code = last.exit;
    exitEl.innerHTML = `exit <b>${last.exit}</b> <span>${t.exit[last.exit]}</span>`;
    if (last.error) out.innerHTML = `<span class="c-r">${esc(last.error)}</span>`;
    else out.innerHTML = view === "json" ? esc(last.json) : ansi(last.text);
  }

  root.querySelector(".out-head [role=tablist]").addEventListener("tabchange", (e) => {
    view = e.detail.dataset.view;
    showCommand(request());
    render();
  });
  chips.forEach((c) => c.addEventListener("click", () => load(c.dataset.preset)));
  [keysOnly, showSecrets].forEach((x) => x.addEventListener("change", changed));
  ignore.addEventListener("input", changed);
  addBtn.addEventListener("click", () => {
    const n = filesEl.children.length + 1;
    const first = filesEl.querySelector("input")?.value || "";
    const ext = fmtOf(first) === "dotenv" ? ".env" : fmtOf(first) === "json" ? ".json" : ".yaml";
    filesEl.append(fileEl(`env${n}${ext}`, ""));
    layout();
    changed();
    filesEl.lastElementChild.querySelector("textarea").focus();
  });

  load("k8s");
  out.innerHTML = `<span class="c-d">${t.loading}</span>`;

  async function boot() {
    try {
      await new Promise((ok, fail) => {
        const s = document.createElement("script");
        s.src = "/assets/wasm_exec.js";
        s.onload = ok; s.onerror = fail;
        document.head.append(s);
      });
      const go = new Go();
      const res = await WebAssembly.instantiateStreaming(fetch("/assets/engine.wasm"), go.importObject);
      go.run(res.instance);
      engine = globalThis.confdrift;
      runNow();
    } catch (err) {
      console.error(err);
      out.innerHTML = `<span class="c-r">${t.failed}</span>`;
    }
  }
  const io = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) { io.disconnect(); boot(); }
  }, { rootMargin: "600px" });
  io.observe(root);
}
