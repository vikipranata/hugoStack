// hugoStack editor — BookStack-style page:
//   WYSIWYG (TinyMCE 7) | Markdown (CodeMirror 6 + live preview, draggable
//   divider, proportional scroll sync).
// Single source of truth: the markdown text. WYSIWYG HTML is converted with
// turndown + gfm plugin on mode switch and on save, so neither mode can lose
// content. Relative uploads/ refs are rewritten to /post/<slug>/uploads/
// for display in both panes and rewritten back before saving.
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

const state = { md: "", mode: "wysiwyg", dirty: false };
let draftLoaded = false;
let cmView = null;

const previewEl = $("editor-preview");
const cmHost = $("cm-host");
const paneMd = $("pane-md");
const divider = $("mdsplit-divider");

function markDirty() { state.dirty = true; }

/* ---------- markdown <-> html (display-side image rewrite) ---------- */

function esc(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

function mdToHtml(md) {
  if (!window.marked) return "<pre>" + esc(md) + "</pre>"; // ponytail: marked missing (should not happen — vendored) -> escaped text
  return marked.parse(md);
}

const PUBLIC_UP = "/post/" + SLUG + "/uploads/";

function htmlForDisplay(html) {
  return html.replace(/(src|href)="uploads\//g, (m, a) => a + '"' + PUBLIC_UP);
}

function htmlForEditor(md) {
  return htmlForDisplay(mdToHtml(md));
}

function rewriteBack(md) {
  return md.split(PUBLIC_UP).join("uploads/");
}

/* ---------- turndown: WYSIWYG HTML -> markdown (save path) ---------- */

const td = new TurndownService({
  headingStyle: "atx",
  bulletListMarker: "-",
  codeBlockStyle: "fenced",
  emDelimiter: "*",
  hr: "---",
});
td.use(turndownPluginGfm.tables);
td.use(turndownPluginGfm.strikethrough);
td.use(turndownPluginGfm.taskListItems);
// ponytail: the vendored gfm plugin emits single-tilde ~text~ (not GFM);
// addRule unshifts, so this later-registered rule wins and yields ~~text~~.
td.addRule("gfmStrikethrough", {
  filter: ["s", "strike", "del"],
  replacement: (content) => "~~" + content + "~~",
});

function htmlToMd(html) {
  return rewriteBack(td.turndown(html));
}

// Current content as markdown, whichever mode is showing.
function currentMd() {
  if (state.mode === "wysiwyg") {
    const ed = tinymce.get("tinymce-src");
    if (ed) return htmlToMd(ed.getContent());
    return state.md;
  }
  if (cmView) return cmView.state.doc.toString();
  return state.md;
}

/* ---------- uploads (paste / drop / button) ---------- */

const IMG_EXT = { "image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif" };

async function uploadFile(file) {
  setStatus("Uploading " + file.name + "…");
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

function publicUrlFor(rel) { // rel = "uploads/x.png"
  return PUBLIC_UP + rel.slice("uploads/".length);
}

/* ---------- TinyMCE (WYSIWYG pane) ---------- */

// vq typography for the editor iframe (mirrors the .markdown rules).
const TMCE_CONTENT_CSS =
  'body { font-family: "Plus Jakarta Sans", Inter, system-ui, sans-serif; font-size: 16px; line-height: 1.5; color: #0F172A; background: #FFFFFF; margin: 0; padding: 16px 20px; max-width: 880px; }' +
  "h1, h2, h3, h4 { line-height: 1.25; margin: 20px 0 12px; font-weight: 700; }" +
  "h1 { font-size: 24px; } h2 { font-size: 20px; } h3 { font-size: 16px; } h4 { font-size: 14px; }" +
  "p { margin: 0 0 12px; }" +
  "a { color: #0f556f; }" +
  "code { font-family: \"JetBrains Mono\", monospace; font-size: 14px; background: #F1F5F9; padding: 2px 6px; border-radius: 8px; }" +
  "pre { background: #F1F5F9; border-radius: 8px; padding: 16px; overflow-x: auto; margin: 0 0 12px; }" +
  "pre code { background: none; padding: 0; }" +
  "ul, ol { margin: 0 0 12px; padding-left: 24px; }" +
  "blockquote { margin: 0 0 12px; padding: 8px 16px; border-left: 3px solid #259dcc; color: #475569; }" +
  "table { margin-bottom: 16px; border-collapse: collapse; }" +
  "th, td { border: 1px solid #E2E8F0; padding: 8px 12px; }" +
  "th { background: #F1F5F9; font-weight: 600; }" +
  "img { max-width: 100%; border-radius: 8px; }" +
  "hr { border: none; border-top: 1px solid #E2E8F0; margin: 16px 0; }";

function initTinyMCE(initialMd) {
  const md = initialMd != null ? initialMd : currentMd();
  // TinyMCE takes its initial content from the textarea.
  $("tinymce-src").value = htmlForEditor(md);
  tinymce.init({
    selector: "#tinymce-src",
    base_url: "/static/vendor/tinymce",
    suffix: ".min",
    license_key: "gpl", // community edition
    skin: "oxide",
    theme: "silver",
    branding: false,
    statusbar: false,
    menubar: false,
    promotion: false,
    content_css: false,
    content_style: TMCE_CONTENT_CSS,
    plugins: "image table link autolink fullscreen code codesample lists media",
    toolbar:
      "undo redo | bold italic underline strikethrough | formatselect | bullist numlist |" +
      " link image table | code codesample | blockquote hr | fullscreen",
    relative_urls: false,
    // Paste/drop images from the OS clipboard -> existing /upload/<slug>
    // endpoint; the returned URL is the public /post/... path so the image
    // renders inside the editor iframe immediately.
    images_upload_handler: (blobInfo) => {
      const blob = blobInfo.blob();
      const ext = IMG_EXT[blob.type] || ".png";
      return uploadFile(new File([blob], "pasted" + ext, { type: blob.type }))
        .then((j) => publicUrlFor(j.path));
    },
    setup: (editor) => {
      editor.on("change setcontent undo redo", () => markDirty());
    },
  });
}

/* ---------- CodeMirror 6 (Markdown pane) ---------- */

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
    doc: initialMd != null ? initialMd : currentMd(),
    parent: cmHost,
    extensions: [
      basicSetup,
      markdown({ base: markdownLanguage }),
      syntaxHighlighting(mdTheme),
      EditorView.lineWrapping,
      cmBaseTheme,
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
    if (state.mode !== "md") return;
    previewEl.innerHTML = htmlForDisplay(mdToHtml(cmView.state.doc.toString()));
  }, 120); // debounce per-keystroke re-render
}

// Proportional scroll sync with a short re-entrancy guard: applying the
// same ratio is idempotent, so the ping-pong dies after one round trip.
// ponytail: no scroll-sync library; upgrade path: codemirror's
// @replit/codemirror-vim-style sync lib if precision matters.
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

/* ---------- mode switching (lossless) ---------- */

const modeBtns = { wysiwyg: $("mode-wysiwyg"), md: $("mode-md") };

function setMode(m) {
  if (m === state.mode) return;
  if (m === "md") {
    const md = currentMd();
    if (!cmView) initCM(md);
    else cmView.dispatch({ changes: { from: 0, to: cmView.state.doc.length, insert: md } });
    schedulePreview();
  } else {
    const md = currentMd();
    if (!tinymce.get("tinymce-src")) initTinyMCE(md);
    else tinymce.get("tinymce-src").setContent(htmlForEditor(md));
  }
  state.mode = m;
  $("pane-wysiwyg").hidden = m !== "wysiwyg";
  paneMd.hidden = m !== "md";
  for (const k of Object.keys(modeBtns)) {
    modeBtns[k].classList.toggle("is-active", k === m);
    modeBtns[k].setAttribute("aria-pressed", String(k === m));
  }
  const tp = $("toggle-preview");
  tp.hidden = m !== "md";
  if (m === "md") requestAnimationFrame(() => cmView && cmView.focus());
  else requestAnimationFrame(() => { const ed = tinymce.get("tinymce-src"); if (ed) ed.focus(); });
}

modeBtns.wysiwyg.addEventListener("click", () => setMode("wysiwyg"));
modeBtns.md.addEventListener("click", () => setMode("md"));

$("toggle-preview").addEventListener("click", () => {
  const hidden = paneMd.classList.toggle("is-preview-hidden");
  const btn = $("toggle-preview");
  btn.textContent = hidden ? "Show preview" : "Hide preview";
  btn.setAttribute("aria-pressed", String(!hidden));
});

/* ---------- draggable divider (20–80%) ---------- */

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

/* ---------- save (both modes -> markdown) ---------- */

async function save() {
  const body = currentMd(); // wysiwyg: turndown-converted; md: verbatim
  setStatus("Saving…");
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

/* ---------- title: inline (top row) <-> sidebar mirror ---------- */

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

/* ---------- image insertion (button / paste / drop, both modes) ---------- */

function insertAtCursor(j) {
  if (state.mode === "wysiwyg" && tinymce.get("tinymce-src")) {
    tinymce.get("tinymce-src").insertContent(
      '<p><img src="' + publicUrlFor(j.path) + '" alt=""></p>'
    );
  } else {
    insertMd(j.snippet);
  }
}

function insertMd(snippet) {
  if (state.mode !== "md") setMode("md");
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
  try { insertAtCursor(await uploadFile(f)); }
  catch (err) { setStatus("Upload failed: " + err.message); }
});

// Markdown pane: paste an image from the clipboard / drop a file.
cmHost.addEventListener("paste", (e) => {
  const items = e.clipboardData && e.clipboardData.items ? Array.from(e.clipboardData.items) : [];
  const img = items.find((i) => i.type && i.type.indexOf("image/") === 0);
  if (!img) return;
  e.preventDefault();
  e.stopPropagation();
  uploadFile(fileFromClipboardItem(img))
    .then(insertAtCursor)
    .catch((err) => setStatus("Upload failed: " + err.message));
}, true);

cmHost.addEventListener("dragover", (e) => { if (imageFileFrom(e)) e.preventDefault(); }, true);
cmHost.addEventListener("drop", (e) => {
  const f = imageFileFrom(e);
  if (!f) return;
  e.preventDefault();
  e.stopPropagation();
  uploadFile(f)
    .then(insertAtCursor)
    .catch((err) => setStatus("Upload failed: " + err.message));
}, true);

// WYSIWYG pane: file drop (TinyMCE already handles clipboard paste via
// images_upload_handler).
const paneWy = $("pane-wysiwyg");
paneWy.addEventListener("dragover", (e) => { if (imageFileFrom(e)) e.preventDefault(); }, true);
paneWy.addEventListener("drop", (e) => {
  const f = imageFileFrom(e);
  if (!f || !tinymce.get("tinymce-src")) return;
  e.preventDefault();
  e.stopPropagation();
  uploadFile(f)
    .then((j) => tinymce.get("tinymce-src").insertContent('<p><img src="' + publicUrlFor(j.path) + '" alt=""></p>'))
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
    setStatus("Draft loaded — not saved yet");
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
    tBtn.innerHTML = "Translating…";
    setStatus("Translating…");
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

// The textarea holds the markdown; only read it when no translated draft
// was stashed for this slug+lang (the draft block above set state.md).
if (!draftLoaded) state.md = $("tinymce-src").value;
initTinyMCE(state.md); // default mode: WYSIWYG
