package main

import (
	"fmt"
	"html/template"
	"os"

	"hugostack/internal/browser"
)

// shellData is the data every page renders through the shell template.
// Optional fields stay zero for pages that don't use them.
type shellData struct {
	Title             string
	BreadcrumbCurrent string
	BannerTitle       string
	BannerCopy        string
	Active            string
	NoNav             bool
	Posts             []browser.Post
	// settings
	HugoPath  string
	Port      int
	AdminUser string
	// editor
	Slug       string
	Lang       string
	FileName   string
	Langs      []string
	IsNew      bool // target file does not exist yet (unsaved draft)
	LLMReady   bool // LLM config complete -> show translate button
	TargetLang string
	// settings (LLM form)
	LLMBaseURL   string
	LLMAPIKey    string
	LLMModel     string
	LLMKeySet    bool
	TitleDisplay string
	TitleValue   string
	TagsCSV      string
	DescValue    string
	BodyText     string
	// git & deploy (Story 4.2)
	GitBranch     string
	GitAhead      int
	GitBehind     int
	GitDirty      int
	GitUpstream   bool
	GitError      string
	BranchPages   string
	BranchRelease string
	GitUser       string
	GitEmail      string
	// shared
	PageJS template.JS
	CSS    template.CSS
}

// pages: one template set per page (shell + its "body" define).
var pages map[string]*template.Template

func newShellData(title, breadcrumb, active string) *shellData {
	return &shellData{Title: title, BreadcrumbCurrent: breadcrumb, Active: active, CSS: pageCSS}
}

// initTemplates loads the shell + page templates and the stylesheet.
// CSS is passed as template.CSS so the <style> block is not escaped
// (a plain string would render ZgotmplZ).
func initTemplates() error {
	css, err := os.ReadFile("ui/static/style.css")
	if err != nil {
		return err
	}
	pages = map[string]*template.Template{}
	for _, n := range []string{"index", "settings", "login", "editor"} {
		pages[n] = template.Must(template.New("").ParseFiles(
			"ui/templates/shell.html",
			fmt.Sprintf("ui/templates/%s.html", n),
		))
	}
	pageCSS = template.CSS(css)
	return nil
}

var pageCSS template.CSS

// editorJS is the editor page script. Args: slug, lang, llmReady, target.
// ponytail: live preview (marked.js CDN) instead of a full WYSIWYG
// rich-text library — no build step, one CDN tag. Upgrade path:
// CodeMirror 6 if syntax highlighting becomes a real need.
const editorJS = `
(function () {
  var SLUG = %q, LANG = %q, LLM_READY = %t, TARGET = %q;
  var src = document.getElementById("editor-src");
  var preview = document.getElementById("editor-preview");
  var bMd = document.getElementById("view-md");
  var bPrev = document.getElementById("view-preview");
  var status = document.getElementById("save-status");
  var view = "md";
  function esc(s) { var d = document.createElement("div"); d.textContent = s; return d.innerHTML; }
  function render() {
    var md = src.value;
    // relative uploads/ links -> served via /post/<slug>/uploads/
    md = md.replace(/src="uploads\//g, 'src="/post/' + SLUG + '/uploads/');
    md = md.replace(/\]\(uploads\//g, '](/post/' + SLUG + '/uploads/');
    preview.innerHTML = window.marked ? marked.parse(md) : esc(md);
  }
  function setView(v) {
    view = v;
    if (v === "preview") {
      render();
      src.hidden = true; preview.hidden = false;
      bPrev.className = "btn btn--primary btn--sm"; bPrev.setAttribute("aria-pressed", "true");
      bMd.className = "btn btn--ghost btn--sm"; bMd.setAttribute("aria-pressed", "false");
    } else {
      src.hidden = false; preview.hidden = true;
      bMd.className = "btn btn--primary btn--sm"; bMd.setAttribute("aria-pressed", "true");
      bPrev.className = "btn btn--ghost btn--sm"; bPrev.setAttribute("aria-pressed", "false");
    }
  }
  bMd.addEventListener("click", function () { setView("md"); });
  bPrev.addEventListener("click", function () { setView("preview"); });
  src.addEventListener("input", function () { if (view === "preview") render(); });

  document.getElementById("save-btn").addEventListener("click", function () {
    status.textContent = "Saving…";
    var fd = new FormData();
    fd.append("lang", LANG);
    fd.append("title", document.getElementById("f-title").value);
    fd.append("tags", document.getElementById("f-tags").value);
    fd.append("description", document.getElementById("f-desc").value);
    fd.append("body", src.value);
    fetch("/edit/" + SLUG, { method: "POST", body: fd, credentials: "same-origin" })
      .then(function (res) {
        if (!res.ok) { return res.text().then(function (t) { throw new Error(t); }); }
        status.textContent = "Saved";
      })
      .catch(function (e) { status.textContent = "Save failed: " + e.message; });
  });

  function insertSnippet(s) {
    if (view === "preview") { setView("md"); }
    var start = src.selectionStart, end = src.selectionEnd;
    src.setRangeText(s, start, end, "end");
    src.focus();
  }
  async function uploadFile(file) {
    status.textContent = "Uploading " + file.name + "…";
    var fd = new FormData();
    fd.append("file", file);
    var res = await fetch("/upload/" + SLUG, { method: "POST", body: fd, credentials: "same-origin" });
    if (!res.ok) { throw new Error(await res.text()); }
    var j = await res.json();
    insertSnippet(j.snippet);
    status.textContent = "Inserted " + j.path;
  }
  document.getElementById("up-btn").addEventListener("click", function () {
    document.getElementById("up-file").click();
  });
  document.getElementById("up-file").addEventListener("change", function (e) {
    if (e.target.files[0]) {
      uploadFile(e.target.files[0]).catch(function (er) { status.textContent = "Upload failed: " + er.message; });
    }
    e.target.value = "";
  });
  var editor = document.getElementById("editor");
  editor.addEventListener("dragover", function (e) { e.preventDefault(); });
  editor.addEventListener("drop", function (e) {
    e.preventDefault();
    var f = Array.from(e.dataTransfer.files).find(function (f) { return f.type.indexOf("image/") === 0; });
    if (f) { uploadFile(f).catch(function (er) { status.textContent = "Upload failed: " + er.message; }); }
  });
  // Translated draft handoff: the translate button stashes the result in
  // sessionStorage and redirects here; load it into the form as an unsaved
  // draft (story 3.3). Nothing is saved until the user hits Save.
  var draftKey = "hugostack-draft:" + SLUG + ":" + LANG;
  var rawDraft = sessionStorage.getItem(draftKey);
  if (rawDraft) {
    sessionStorage.removeItem(draftKey);
    try {
      var d = JSON.parse(rawDraft);
      document.getElementById("f-title").value = d.title || "";
      document.getElementById("f-tags").value = (d.tags || []).join(", ");
      document.getElementById("f-desc").value = d.description || "";
      src.value = d.body || "";
      var db = document.getElementById("draft-banner");
      if (db) {
        db.hidden = false;
        var bt = db.querySelector(".banner__title");
        if (bt) { bt.textContent = "Translated draft — review then Save"; }
      }
      status.textContent = "Draft loaded — not saved yet";
      if (view === "preview") { render(); }
    } catch (e) { status.textContent = "Draft load failed: " + e.message; }
  }

  // paste image from clipboard -> base64 -> uploads/
  src.addEventListener("paste", function (e) {
    var items = e.clipboardData ? e.clipboardData.items : [];
    for (var i = 0; i < items.length; i++) {
      if (items[i].type.indexOf("image/") === 0) {
        e.preventDefault();
        var blob = items[i].getAsFile();
        var ext = { "image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif" }[blob.type] || ".png";
        uploadFile(new File([blob], "pasted" + ext, { type: blob.type })).catch(function (er) { status.textContent = "Upload failed: " + er.message; });
        return;
      }
    }
  });

  // Story 3.3: translate button (rendered only when LLM config is set).
  // POSTs to /api/translate/<slug>, stashes the returned draft, then opens
  // the target-lang editor which loads it as an unsaved draft. No auto-save.
  var tBtn = document.getElementById("translate-btn");
  if (tBtn) {
    tBtn.addEventListener("click", function () {
      var orig = tBtn.innerHTML;
      tBtn.disabled = true;
      tBtn.innerHTML = '<span class="spinner" aria-hidden="true"></span>Translating…';
      status.textContent = "Translating…";
      fetch("/api/translate/" + SLUG + "?lang=" + LANG, { method: "POST", credentials: "same-origin" })
        .then(function (res) {
          if (!res.ok) { return res.text().then(function (t) { throw new Error(t); }); }
          return res.json();
        })
        .then(function (d) {
          sessionStorage.setItem("hugostack-draft:" + SLUG + ":" + d.lang, JSON.stringify(d));
          location.href = "/edit/" + SLUG + "?lang=" + d.lang;
        })
        .catch(function (e) {
          tBtn.disabled = false;
          tBtn.innerHTML = orig;
          status.textContent = "Translate failed: " + e.message;
        });
    });
  }
})();
`

// deployJS is the Git & Deploy panel script. Args: pagesRef, releaseRef.
// Status is server-rendered on page load, then polled (ponytail: 20s poll
// = "live" for a single-user admin UI; upgrade path: SSE if it gets chatty).
const deployJS = `
(function () {
  var PAGES_REF = %q, RELEASE_REF = %q;
  var line = document.getElementById("git-status-line");
  var out = document.getElementById("git-output");
  if (!line) { return; }
  var dirty = 0;
  function showOut(t, ok) {
    out.hidden = false;
    out.textContent = t;
    out.style.color = ok === false ? "var(--style-status-danger-text)" : "";
  }
  function refreshStatus() {
    fetch("/api/git/status", { credentials: "same-origin" })
      .then(function (r) { return r.text().then(function (t) {
        try { return JSON.parse(t); } catch (e) { return { ok: false, error: t }; }
      }); })
      .then(function (d) {
        if (!d.ok) { line.textContent = "git: " + (d.error || "status failed"); return; }
        dirty = d.dirty;
        var up = d.upstream ? ("ahead " + d.ahead + ", behind " + d.behind) : "no upstream";
        line.textContent = d.branch + " · " + up + " · " + d.dirty + " file(s) modified";
      })
      .catch(function (e) { line.textContent = "status failed: " + e.message; });
  }
  function api(url, body) {
    var opt = { method: "POST", credentials: "same-origin" };
    if (body) {
      opt.headers = { "Content-Type": "application/json" };
      opt.body = JSON.stringify(body);
    }
    return fetch(url, opt).then(function (res) {
      return res.text().then(function (t) {
        try { return JSON.parse(t); } catch (e) { return { ok: false, output: t }; }
      });
    });
  }
  function run(btn, label, p) {
    var orig = btn.textContent;
    btn.disabled = true;
    btn.textContent = "Working…";
    showOut(label + "…", undefined);
    return p.then(function (d) {
      showOut(d.output || "(no output)", d.ok);
      refreshStatus();
      return d;
    }).catch(function (e) {
      showOut(String(e), false);
    }).then(function (d) {
      btn.disabled = false;
      btn.textContent = orig;
      return d;
    });
  }
  var msg = document.getElementById("commit-msg");
  document.getElementById("btn-commit").addEventListener("click", function () {
    run(this, "Committing", api("/api/git/commit", { message: msg.value.trim() }));
  });
  document.getElementById("btn-pages").addEventListener("click", function () {
    run(this, "Pushing to " + PAGES_REF, api("/api/git/push?ref=" + encodeURIComponent(PAGES_REF)));
  });
  // Release = commit pending changes (skipped when the tree is clean),
  // then push the release branch.
  document.getElementById("btn-release").addEventListener("click", function () {
    var self = this;
    var p = Promise.resolve({ ok: true });
    if (dirty > 0) {
      var m = msg.value.trim() || ("Release " + new Date().toISOString().slice(0, 16).replace("T", " "));
      p = api("/api/git/commit", { message: m });
    }
    run(self, "Releasing to " + RELEASE_REF, p.then(function (d) {
      if (!d.ok) { return d; }
      return api("/api/git/push?ref=" + encodeURIComponent(RELEASE_REF));
    }));
  });
  document.getElementById("btn-refresh").addEventListener("click", refreshStatus);
  refreshStatus();
  setInterval(refreshStatus, 20000);
})();
`
