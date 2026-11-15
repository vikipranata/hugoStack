// hugoStack editor — markdown-only (BookStack-style page):
//   CodeMirror 6 + live preview, draggable divider (20-80%), proportional
//   scroll sync, post-details panel on the left.
// Single source of truth: the markdown text (CodeMirror doc). Relative
// uploads/ refs are rewritten to /post/<slug>/uploads/ for display in the
// preview and rewritten back before saving.
import {
  EditorView,
  basicSetup,
  markdown,
  markdownLanguage,
  HighlightStyle,
  syntaxHighlighting,
  tags as t,
} from "/static/vendor/cm.js";

const CFG = JSON.parse(document.getElementById("editor-config").textContent);
const SLUG = CFG.slug;
const LANG = CFG.lang;

const $ = (id) => document.getElementById(id);
const statusEl = $("save-status");
const setStatus = (msg) => { statusEl.textContent = msg; };

const state = { md: "", dirty: false };
let draftLoaded = false;
let cmView = null;

const previewEl = $("editor-preview");
const cmHost = $("cm-host");
const paneMd = $("pane-md");
const divider = $("mdsplit-divider");

function markDirty() { state.dirty = true; }

/* ---------- markdown -> html (display-side image rewrite) ---------- */

function esc(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

function mdToHtml(md) {
  if (!window.marked) return "<pre>" + esc(md) + "</pre>"; // ponytail: marked missing (should not happen - vendored) -> escaped text
  return marked.parse(md);
}

const PUBLIC_UP = "/post/" + SLUG + "/uploads/";

function htmlForDisplay(md) {
  return mdToHtml(md).replace(/(src|href)="uploads\//g, (m, a) => a + '"' + PUBLIC_UP);
}

function rewriteBack(md) {
  return md.split(PUBLIC_UP).join("uploads/");
}

// Current content as markdown (the CodeMirror doc).
function currentMd() {
  if (cmView) return rewriteBack(cmView.state.doc.toString());
  return state.md;
}

/* ---------- uploads (paste / drop / button) ---------- */

const IMG_EXT = { "image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif" };

async function uploadFile(file) {
  setStatus("Uploading " + file.name + "...");
  const fd = new FormData();
  fd.append("file", file);
  const res = await fetch("/upload/" + SLUG, { method: "POST", body: fd, credentials: "same-origin" });
  if (!res.ok) throw new Error(await res.text());
  const j = await res.json(); // {path: "uploads/<name>", snippet: "![..](uploads/<name>)"}
  setStatus("Inserted " + j.path);
  return j;
}

function imageFileFrom(e) {
  const files = e.dataTransfer && e.dataTransfer.files ? Array.from(e.dataTransfer.files) : [];
  return files.find((f) => f.type.indexOf("image/") === 0) || null;
}

function fileFromClipboardItem(item) {
  const blob = item.getAsFile();
  const ext = IMG_EXT[blob.type] || ".png";
  return new File([blob], "pasted" + ext, { type: blob.type });
}

/* ---------- CodeMirror 6 (the editor) ---------- */

const mdTheme = HighlightStyle.define([
  { tag: [t.heading1, t.heading2, t.heading3, t.heading4], fontWeight: "700", color: "#0f556f" },
  { tag: t.heading1, fontSize: "1.25em" },
  { tag: t.heading2, fontSize: "1.1em" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "700" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: [t.monospace, t.literal], fontFamily: '"JetBrains Mono", monospace', color: "#475569" },
  { tag: t.quote, color: "#64748B" },
  { tag: [t.link, t.url], color: "#259dcc" },
]);

// ponytail: hex values mirror the --style-* tokens in :root (CM HighlightStyle
// specs are easier to read as literals); keep them in sync manually.
const cmBaseTheme = EditorView.theme({
  "&": { height: "100%", fontSize: "14px" },
  ".cm-scroller": {
    fontFamily: '"JetBrains Mono", SFMono-Regular, Consolas, monospace',
    lineHeight: "1.6",
  },
  ".cm-content": { padding: "16px 0", caretColor: "#0f556f" },
  ".cm-line": { padding: "0 12px 0 6px" },
  ".cm-gutters": { backgroundColor: "#F8FAFC", color: "#64748B", borderRight: "1px solid #E2E8F0" },
  ".cm-activeLine": { backgroundColor: "transparent" },
  ".cm-activeLineGutter": { backgroundColor: "#F1F5F9" },
  "&.cm-focused": { outline: "none" },
  ".cm-selectionBackground": { backgroundColor: "#E9FBF6" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "#0f556f" },
});

function initCM(initialMd) {
  cmView = new EditorView({
    doc: initialMd != null ? initialMd : state.md,
    parent: cmHost,
    extensions: [
      basicSetup,
      markdown({ base: markdownLanguage }),
      syntaxHighlighting(mdTheme),
      EditorView.lineWrapping,
      cmBaseTheme,
      // ponytail: current @codemirror/view exposes updateListener as a Facet,
      // not a callable static - .of(fn) is the registered shape.
      EditorView.updateListener.of((u) => {
        if (u.docChanged) {
          state.md = u.state.doc.toString();
          markDirty();
          schedulePreview();
          syncScroll(cmView.scrollDOM, previewEl);
        }
      }),
    ],
  });
  cmView.scrollDOM.addEventListener("scroll", () => syncScroll(cmView.scrollDOM, previewEl));
}

/* ---------- live preview + scroll sync ---------- */

let previewTimer = 0;
function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(() => {
    if (!cmView) return;
    previewEl.innerHTML = htmlForDisplay(cmView.state.doc.toString());
  }, 120); // debounce per-keystroke re-render
}

// Proportional scroll sync with a short re-entrancy guard: applying the
// same ratio is idempotent, so the ping-pong dies after one round trip.
let lastSyncAt = 0;
function syncScroll(from, to) {
  const now = performance.now();
  if (now - lastSyncAt < 80) return;
  lastSyncAt = now;
  const max = from.scrollHeight - from.clientHeight;
  if (max <= 0) return;
  to.scrollTop = (from.scrollTop / max) * (to.scrollHeight - to.clientHeight);
}

previewEl.addEventListener("scroll", () => {
  if (cmView) syncScroll(previewEl, cmView.scrollDOM);
});

$("toggle-preview").addEventListener("click", () => {
  const hidden = paneMd.classList.toggle("is-preview-hidden");
  const btn = $("toggle-preview");
  btn.textContent = hidden ? "Show preview" : "Hide preview";
  btn.setAttribute("aria-pressed", String(!hidden));
});

/* ---------- draggable divider (20-80%) ---------- */

function setRatio(pct) {
  const clamped = Math.min(80, Math.max(20, pct));
  paneMd.style.setProperty("--mdsplit-ratio", clamped + "%");
}

divider.addEventListener("pointerdown", (e) => {
  e.preventDefault();
  divider.setPointerCapture(e.pointerId);
  divider.classList.add("is-dragging");
  const move = (ev) => {
    const rect = paneMd.getBoundingClientRect();
    if (rect.width <= 0) return;
    setRatio(((ev.clientX - rect.left) / rect.width) * 100);
  };
  const up = () => {
    divider.classList.remove("is-dragging");
    divider.removeEventListener("pointermove", move);
    divider.removeEventListener("pointerup", up);
    divider.removeEventListener("pointercancel", up);
  };
  divider.addEventListener("pointermove", move);
  divider.addEventListener("pointerup", up);
  divider.addEventListener("pointercancel", up);
});

divider.addEventListener("keydown", (e) => {
  if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
  e.preventDefault();
  const cur = parseFloat(getComputedStyle(paneMd).getPropertyValue("--mdsplit-ratio")) || 50;
  setRatio(cur + (e.key === "ArrowRight" ? 2 : -2));
});

/* ---------- save (markdown, verbatim) ---------- */

async function save() {
  const body = currentMd();
  setStatus("Saving...");
  const fd = new FormData();
  fd.append("lang", LANG);
  fd.append("title", $("f-title").value);
  fd.append("tags", $("f-tags").value);
  fd.append("description", $("f-desc").value);
  fd.append("body", body);
  try {
    const res = await fetch("/edit/" + SLUG, { method: "POST", body: fd, credentials: "same-origin" });
    if (!res.ok) throw new Error(await res.text());
    state.dirty = false;
    setStatus("Saved");
  } catch (err) {
    setStatus("Save failed: " + err.message);
  }
}

$("save-btn").addEventListener("click", save);
document.addEventListener("keydown", (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
    e.preventDefault();
    save();
  }
});

window.addEventListener("beforeunload", (e) => {
  if (state.dirty) {
    e.preventDefault();
    e.returnValue = "";
  }
});

/* ---------- title: inline (top row) <-> panel mirror ---------- */

const titleInline = $("title-inline");
const fTitle = $("f-title");

titleInline.addEventListener("input", () => {
  fTitle.value = titleInline.textContent.replace(/\s+$/, "");
  markDirty();
});
titleInline.addEventListener("keydown", (e) => {
  if (e.key === "Enter") { e.preventDefault(); titleInline.blur(); } // single-line
});
titleInline.addEventListener("paste", (e) => {
  e.preventDefault(); // plain text only
  const text = (e.clipboardData || window.clipboardData).getData("text/plain");
  document.execCommand("insertText", false, text);
});
fTitle.addEventListener("input", () => { titleInline.textContent = fTitle.value; });
["f-desc", "f-tags"].forEach((id) => $(id).addEventListener("input", markDirty));

/* ---------- image insertion (button / paste / drop) ---------- */

function insertMd(snippet) {
  if (!cmView) return;
  const from = cmView.state.selection.main.from;
  cmView.dispatch({
    changes: { from, insert: snippet },
    selection: { anchor: from + snippet.length },
  });
  cmView.focus();
}

$("up-btn").addEventListener("click", () => $("up-file").click());
$("up-file").addEventListener("change", async (e) => {
  const f = e.target.files[0];
  e.target.value = "";
  if (!f) return;
  try { insertMd((await uploadFile(f)).snippet); }
  catch (err) { setStatus("Upload failed: " + err.message); }
});

// Paste an image from the clipboard / drop a file onto the editor.
cmHost.addEventListener("paste", (e) => {
  const items = e.clipboardData && e.clipboardData.items ? Array.from(e.clipboardData.items) : [];
  const img = items.find((i) => i.type && i.type.indexOf("image/") === 0);
  if (!img) return;
  e.preventDefault();
  e.stopPropagation();
  uploadFile(fileFromClipboardItem(img))
    .then((j) => insertMd(j.snippet))
    .catch((err) => setStatus("Upload failed: " + err.message));
}, true);

cmHost.addEventListener("dragover", (e) => { if (imageFileFrom(e)) e.preventDefault(); }, true);
cmHost.addEventListener("drop", (e) => {
  const f = imageFileFrom(e);
  if (!f) return;
  e.preventDefault();
  e.stopPropagation();
  uploadFile(f)
    .then((j) => insertMd(j.snippet))
    .catch((err) => setStatus("Upload failed: " + err.message));
}, true);

/* ---------- translated-draft handoff (Story 3.3) ---------- */

const draftKey = "hugostack-draft:" + SLUG + ":" + LANG;
const rawDraft = sessionStorage.getItem(draftKey);
if (rawDraft) {
  sessionStorage.removeItem(draftKey);
  try {
    const d = JSON.parse(rawDraft);
    fTitle.value = d.title || "";
    titleInline.textContent = fTitle.value;
    $("f-tags").value = (d.tags || []).join(", ");
    $("f-desc").value = d.description || "";
    state.md = d.body || "";
    draftLoaded = true;
    const db = $("draft-banner");
    if (db) db.hidden = false;
    setStatus("Draft loaded - not saved yet");
    markDirty();
  } catch (err) {
    setStatus("Draft load failed: " + err.message);
  }
}

/* ---------- translate button (Story 3.3) ---------- */

const tBtn = $("translate-btn");
if (tBtn) {
  tBtn.addEventListener("click", () => {
    const orig = tBtn.innerHTML;
    tBtn.disabled = true;
    tBtn.innerHTML = "Translating...";
    setStatus("Translating...");
    fetch("/api/translate/" + SLUG + "?lang=" + LANG, { method: "POST", credentials: "same-origin" })
      .then((res) => {
        if (!res.ok) return res.text().then((tx) => { throw new Error(tx); });
        return res.json();
      })
      .then((d) => {
        sessionStorage.setItem("hugostack-draft:" + SLUG + ":" + d.lang, JSON.stringify(d));
        location.href = "/edit/" + SLUG + "?lang=" + d.lang;
      })
      .catch((err) => {
        tBtn.disabled = false;
        tBtn.innerHTML = orig;
        setStatus("Translate failed: " + err.message);
      });
  });
}

/* ---------- boot ---------- */

// The hidden md-src textarea holds the markdown as server-rendered it; only
// read it when no translated draft was stashed for this slug+lang (the draft
// block above set state.md).
if (!draftLoaded) state.md = $("md-src").value;
initCM(state.md);
schedulePreview();
