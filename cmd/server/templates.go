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
	Slug         string
	Lang         string
	FileName     string
	Langs        []string
	TitleDisplay string
	TitleValue   string
	TagsCSV      string
	DescValue    string
	BodyText     string
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

// editorJS is the editor page script. Args: slug, lang.
// ponytail: live preview (marked.js CDN) instead of a full WYSIWYG
// rich-text library — no build step, one CDN tag. Upgrade path:
// CodeMirror 6 if syntax highlighting becomes a real need.
const editorJS = `
(function () {
  var SLUG = %q, LANG = %q;
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
})();
`
