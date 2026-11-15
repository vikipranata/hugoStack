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
	// MissingLangs: languages without a file yet -> "+ lang" add buttons.
	// Computed in Go: html/template has no `in` function (its FuncMap
	// predates Go 1.12's in).
	MissingLangs []string
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
	EditorConfigJSON template.JS
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

// deployJS is the Git & Deploy panel script. Args: pagesRef (the deploy
// branch; push target for "Push").
// Status is server-rendered on page load, then polled (ponytail: 20s poll
// = "live" for a single-user admin UI; upgrade path: SSE if it gets chatty).
const deployJS = `
(function () {
  var PAGES_REF = %q;
  var line = document.getElementById("git-status-line");
  var out = document.getElementById("git-output");
  if (!line) { return; }
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
  document.getElementById("btn-refresh").addEventListener("click", refreshStatus);
  refreshStatus();
  setInterval(refreshStatus, 20000);
})();
`
