package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hugostack/internal/auth"
	"hugostack/internal/browser"
	"hugostack/internal/config"
	"hugostack/internal/git"
	"hugostack/internal/llm"
)

var cfg *config.Config

var langFiles = map[string]string{"en": "index.md", "id": "index.id.md"}

func main() {
	bootstrapConfig()
	var err error
	cfg, err = config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := initTemplates(); err != nil {
		log.Fatalf("templates: %v", err)
	}

	http.HandleFunc("/login", handleLogin)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./ui/static"))))

	// Protected routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/settings", handleSettings)
	mux.HandleFunc("/api/posts", handleAPIPosts)
	mux.HandleFunc("/api/posts/", handleAPISinglePost)
	mux.HandleFunc("/api/translate/", handleTranslate)
	mux.HandleFunc("/api/git/status", handleGitStatus)
	mux.HandleFunc("/api/git/commit", handleGitCommit)
	mux.HandleFunc("/api/git/push", handleGitPush)
	mux.HandleFunc("/edit/", handleEdit)
	mux.HandleFunc("/post/", handlePostAsset)
	mux.HandleFunc("/upload/", handleUpload)
	http.Handle("/", auth.Middleware(mux))

	fmt.Printf("HugoStack starting on :%d (hugo: %s)\n", cfg.Port, cfg.HugoPath)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), nil))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		user := r.FormValue("user")
		pass := r.FormValue("pass")
		if user == cfg.AdminUser && auth.CheckPassword(cfg.AdminPass, pass) {
			http.SetCookie(w, &http.Cookie{Name: "session_id", Value: auth.SessionToken(), Path: "/", HttpOnly: true})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	data := newShellData("Login", "", "")
	data.NoNav = true
	pages["login"].ExecuteTemplate(w, "shell", data)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	posts, err := browser.ListPosts(cfg.HugoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newShellData("Posts", "Posts", "posts")
	data.Posts = posts
	data.BranchPages, data.BranchRelease = cfg.BranchPages, cfg.BranchRelease
	if st, err := git.Status(cfg.HugoPath, cfg.GitUser, cfg.GitEmail); err != nil {
		data.GitError = err.Error()
	} else {
		data.GitBranch, data.GitAhead, data.GitBehind, data.GitDirty = st.Branch, st.Ahead, st.Behind, st.DirtyFiles
		data.GitUpstream = st.HasUpstream
	}
	data.PageJS = template.JS(fmt.Sprintf(deployJS, cfg.BranchPages, cfg.BranchRelease))
	pages["index"].ExecuteTemplate(w, "shell", data)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		base := strings.TrimSpace(r.FormValue("llm_base_url"))
		model := strings.TrimSpace(r.FormValue("llm_model"))
		key := r.FormValue("llm_api_key")
		if base != "" && !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
			http.Error(w, "llm_base_url must start with http:// or https://", http.StatusBadRequest)
			return
		}
		cfg.LLMBaseURL, cfg.LLMModel = base, model
		if key != "" { // empty key field = keep current key (masked in the form)
			cfg.LLMAPIKey = key
		}
		bp := strings.TrimSpace(r.FormValue("branch_pages"))
		br := strings.TrimSpace(r.FormValue("branch_release"))
		if bp != "" && !git.ValidRef(bp) {
			http.Error(w, "branch_pages: invalid ref", http.StatusBadRequest)
			return
		}
		if br != "" && !git.ValidRef(br) {
			http.Error(w, "branch_release: invalid ref", http.StatusBadRequest)
			return
		}
		if bp != "" {
			cfg.BranchPages = bp
		}
		if br != "" {
			cfg.BranchRelease = br
		}
		cfg.GitUser = strings.TrimSpace(r.FormValue("git_user"))
		cfg.GitEmail = strings.TrimSpace(r.FormValue("git_email"))
		if err := config.Save(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	data := newShellData("Settings", "Settings", "settings")
	data.BannerTitle = "Default admin password is \"admin\" (or HUGOSTACK_ADMIN_PASS in Docker)"
	data.BannerCopy = "Change it: run `go run ./cmd/setpass 'newpass'` and put the printed hash into config.json (admin_pass), then restart. In Docker, first-run bootstrap reads HUGOSTACK_ADMIN_PASS."
	data.HugoPath, data.Port, data.AdminUser = cfg.HugoPath, cfg.Port, cfg.AdminUser
	data.LLMBaseURL, data.LLMModel, data.LLMKeySet = cfg.LLMBaseURL, cfg.LLMModel, cfg.LLMAPIKey != ""
	data.LLMReady = llmReady()
	data.BranchPages, data.BranchRelease, data.GitUser, data.GitEmail = cfg.BranchPages, cfg.BranchRelease, cfg.GitUser, cfg.GitEmail
	pages["settings"].ExecuteTemplate(w, "shell", data)
}

// ---- read/write API (Story 2.3) ----

type postJSON struct {
	Slug        string   `json:"slug"`
	FileName    string   `json:"file"`
	Lang        string   `json:"lang"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
}

func handleAPIPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := browser.ListPosts(cfg.HugoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, posts)
}

func handleAPISinglePost(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/posts/")
	if slug == "" || strings.Contains(slug, "/") {
		http.Error(w, "bad slug", http.StatusBadRequest)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	if _, ok := langFiles[lang]; !ok {
		http.Error(w, "lang must be en or id", http.StatusBadRequest)
		return
	}
	path, ok := postFile(cfg.HugoPath, slug, lang)
	if !ok {
		http.Error(w, "bad slug or lang", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		raw, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, "post not found", http.StatusNotFound)
			return
		}
		_, body, _ := browser.Split(string(raw))
		writeJSON(w, postJSON{
			Slug: slug, FileName: langFiles[lang], Lang: lang,
			Title: browser.TitleOf(string(raw)), Tags: browser.TagsOf(string(raw)),
			Description: fmDescription(string(raw)), Body: body,
		})
	case http.MethodPut:
		// No read here on purpose: a missing file is a valid target —
		// savePost seeds it from the other language (Story 3.2).
		var in struct {
			Title       string          `json:"title"`
			Tags        json.RawMessage `json:"tags"`
			Description string          `json:"description"`
			Body        string          `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		tags, err := decodeTags(in.Tags)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := savePost(slug, lang, in.Title, in.Description, in.Body, tags); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func fmDescription(content string) string {
	v, _ := browser.FMField(content, "description")
	return browser.Unquote(v)
}

func decodeTags(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, errors.New("tags must be an array or comma string")
	}
	return splitTagsCSV(s), nil
}

func savePost(slug, lang, title, desc, body string, tags []string) error {
	path, ok := postFile(cfg.HugoPath, slug, lang)
	if !ok {
		return errors.New("bad slug or lang")
	}
	orig, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// Target file missing (first save, e.g. a fresh translation):
		// seed from the other-language file so date/modified survive;
		// fall back to a bare scaffold.
		other := filepath.Join(filepath.Dir(path), langFiles[otherLang(lang)])
		if b, err := os.ReadFile(other); err == nil {
			orig = b
		} else {
			orig = []byte("---\ntitle: \"\"\ndate: " + time.Now().Format("2006-01-02") + "\n---\n")
		}
	}
	out, err := browser.Rewrite(string(orig), browser.Frontmatter{Title: title, Tags: tags, Description: desc}, body)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, out)
}

// writeFileAtomic: temp file + rename in the same dir, so a crash mid-write
// never leaves a half-written post.
func writeFileAtomic(path, content string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".hugostack-tmp-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// ---- editor UI (Story 2.2) ----

func handleEdit(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/edit/")
	if slug == "" || strings.Contains(slug, "/") {
		http.Error(w, "bad slug", http.StatusBadRequest)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	if _, ok := langFiles[lang]; !ok {
		http.Error(w, "lang must be en or id", http.StatusBadRequest)
		return
	}
	path, ok := postFile(cfg.HugoPath, slug, lang)
	if !ok {
		http.Error(w, "bad slug or lang", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodPost {
		tags := splitTagsCSV(r.FormValue("tags"))
		if err := savePost(slug, lang, r.FormValue("title"), r.FormValue("description"), r.FormValue("body"), tags); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/edit/%s?lang=%s", slug, lang), http.StatusSeeOther)
		return
	}

	raw, err := os.ReadFile(path)
	isNew := false
	if err != nil {
		// Story 3.2: missing target file is a valid state — open an
		// empty draft editor; Save (savePost) creates the file.
		if !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		isNew = true
	}
	_, body, _ := browser.Split(string(raw))
	langList := []string{}
	for _, l := range []string{"en", "id"} {
		if (isNew && l == lang) || fileExists(filepath.Join(filepath.Dir(path), langFiles[l])) {
			langList = append(langList, l)
		}
	}
	data := newShellData("Edit "+slug, slug, "posts")
	data.Slug = slug
	data.Lang = lang
	data.FileName = langFiles[lang]
	data.Langs = langList
	data.TitleDisplay = browser.TitleOf(string(raw))
	data.TitleValue = browser.TitleOf(string(raw))
	data.TagsCSV = strings.Join(browser.TagsOf(string(raw)), ", ")
	data.DescValue = fmDescription(string(raw))
	data.BodyText = body
	data.IsNew = isNew
	data.LLMReady = llmReady()
	data.TargetLang = otherLang(lang)
	if isNew {
		data.TitleDisplay = "New " + lang + " draft"
	}
	data.PageJS = template.JS(fmt.Sprintf(editorJS, slug, lang, llmReady(), otherLang(lang)))
	pages["editor"].ExecuteTemplate(w, "shell", data)
}

func splitTagsCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// handlePostAsset serves a post's page-bundle assets so the editor preview
// can display them: /post/<slug>/uploads/<name>.
func handlePostAsset(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/post/")
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[1] != "uploads" {
		http.NotFound(w, r)
		return
	}
	dir, ok := postDir(cfg.HugoPath, parts[0])
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := parts[2]
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(dir, "uploads", name))
}

// ---- asset upload (Story 2.4) ----

var uploadExts = map[string]bool{"png": true, "jpg": true, "jpeg": true, "webp": true, "gif": true}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/upload/")
	if slug == "" || strings.Contains(slug, "/") {
		http.Error(w, "bad slug", http.StatusBadRequest)
		return
	}
	dir, ok := postDir(cfg.HugoPath, slug)
	if !ok {
		http.Error(w, "bad slug", http.StatusBadRequest)
		return
	}
	upDir := filepath.Join(dir, "uploads")

	var name string
	var data []byte
	switch {
	case strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/"):
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			http.Error(w, "bad multipart: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()
		name = header.Filename
		data, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		var in struct {
			Name string `json:"name"`
			Data string `json:"data"` // base64, for paste/drop
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		dec, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil {
			http.Error(w, "invalid base64", http.StatusBadRequest)
			return
		}
		name, data = in.Name, dec
	}

	name = safeFileName(name)
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if !uploadExts[ext] {
		http.Error(w, "only png, jpg, webp, gif allowed", http.StatusBadRequest)
		return
	}
	// unique name
	final := name
	for n := 1; fileExists(filepath.Join(upDir, final)); n++ {
		final = strings.TrimSuffix(name, "."+ext) + fmt.Sprintf("-%d.%s", n, ext)
	}
	if err := os.MkdirAll(upDir, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeFileAtomic(filepath.Join(upDir, final), string(data)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rel := "uploads/" + final
	alt := strings.TrimSuffix(final, "."+ext)
	writeJSON(w, map[string]string{"path": rel, "snippet": fmt.Sprintf("![%s](%s)", alt, rel)})
}

func safeFileName(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, name)
	if name == "" || name == "." || name == ".." {
		return "image.png"
	}
	return name
}

// ---- LLM translation (Story 3.2) ----
//
// POST /api/translate/<slug>?lang=en -> full translated target-language
// markdown (frontmatter + body) as JSON. Nothing is written to disk: the
// editor loads it as an unsaved draft and the normal save flow persists it.

func llmReady() bool {
	return cfg.LLMBaseURL != "" && cfg.LLMAPIKey != "" && cfg.LLMModel != ""
}

var langNames = map[string]string{"en": "English", "id": "Indonesian"}

func otherLang(l string) string {
	if l == "en" {
		return "id"
	}
	return "en"
}

func translateSystemPrompt(from, to string) string {
	return "You are a strict technical translator. Translate the provided Markdown document " +
		"(a Hugo post with YAML frontmatter) from " + langNames[from] + " to " + langNames[to] + ". Rules: " +
		"- Translate only the frontmatter fields \"title\" and \"description\". Keep \"tags\" untranslated. " +
		"Keep \"date\", \"modified\" and every other frontmatter field exactly as-is. " +
		"- Translate prose only. Keep ALL Markdown syntax exactly: headings, lists, emphasis, code blocks " +
		"(do not translate code inside them), tables, link URLs, image paths and references, HTML comments, " +
		"frontmatter delimiters. Translate link text and image alt text. " +
		"- Do not add, remove or reorder sections. No commentary, no preamble, no notes. " +
		"- Output the complete translated document: start with the first \"---\" frontmatter line and end " +
		"with the last body line. Do not wrap the output in a markdown code fence."
}

// stripOuterFence removes one wrapping ``` fence if the model added one.
func stripOuterFence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return s
	}
	nl := strings.IndexByte(t, '\n')
	if nl == -1 {
		return s
	}
	tail := strings.TrimSuffix(t[nl+1:], "\n")
	if strings.HasSuffix(tail, "```") {
		return strings.TrimRight(tail[:len(tail)-3], "\n")
	}
	return s
}

func handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/api/translate/")
	if slug == "" || strings.Contains(slug, "/") {
		http.Error(w, "bad slug", http.StatusBadRequest)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	if _, ok := langFiles[lang]; !ok {
		http.Error(w, "lang must be en or id", http.StatusBadRequest)
		return
	}
	if !llmReady() {
		http.Error(w, "LLM not configured — set base URL, API key and model in Settings", http.StatusServiceUnavailable)
		return
	}
	path, ok := postFile(cfg.HugoPath, slug, lang)
	if !ok {
		http.Error(w, "bad slug or lang", http.StatusBadRequest)
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	target := otherLang(lang)
	out, err := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel).Chat(r.Context(), []llm.Message{
		{Role: "system", Content: translateSystemPrompt(lang, target)},
		{Role: "user", Content: string(raw)},
	})
	if err != nil {
		http.Error(w, "translation failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	out = stripOuterFence(out)
	if _, _, ok := browser.Split(out); !ok {
		// Refuse to hand back a doc without frontmatter: the save flow
		// (browser.Rewrite) requires it, and a silent fallback would
		// quietly lose the translated title/description.
		http.Error(w, "LLM response missing frontmatter", http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"slug": slug, "lang": target,
		"content":     out,
		"title":       browser.TitleOf(out),
		"tags":        browser.TagsOf(out),
		"description": fmDescription(out),
		"body":        bodyOf(out),
	})
}

func bodyOf(content string) string {
	_, body, _ := browser.Split(content)
	return body
}

// ---- shared helpers ----

func postDir(root, slug string) (string, bool) {
	if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, `/\`) {
		return "", false
	}
	return filepath.Join(root, "content", "posts", slug), true
}

func postFile(root, slug, lang string) (string, bool) {
	f, ok := langFiles[lang]
	if !ok {
		return "", false
	}
	d, ok := postDir(root, slug)
	if !ok {
		return "", false
	}
	return filepath.Join(d, f), true
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ---- git & deploy (Story 4.2/4.3) ----
//
// The deploy panel renders git output in a .output block; every handler
// returns JSON {ok, output} and hands git's combined stdout+stderr
// through verbatim (users need to see auth/SSH failures).

func handleGitStatus(w http.ResponseWriter, r *http.Request) {
	st, err := git.Status(cfg.HugoPath, cfg.GitUser, cfg.GitEmail)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "branch": st.Branch, "dirty": st.DirtyFiles,
		"ahead": st.Ahead, "behind": st.Behind, "upstream": st.HasUpstream,
	})
}

func handleGitCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.Message) == "" {
		http.Error(w, "commit message required", http.StatusBadRequest)
		return
	}
	res := git.Commit(cfg.HugoPath, in.Message, cfg.GitUser, cfg.GitEmail)
	writeJSON(w, map[string]any{"ok": res.OK, "output": res.Output})
}

func handleGitPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ref := r.URL.Query().Get("ref")
	if !git.ValidRef(ref) {
		http.Error(w, "invalid ref", http.StatusBadRequest)
		return
	}
	res := git.Push(cfg.HugoPath, ref, cfg.GitUser, cfg.GitEmail)
	writeJSON(w, map[string]any{"ok": res.OK, "output": res.Output})
}

// ---- first-run bootstrap (Story 4.4) ----

// bootstrapConfig: Docker out-of-the-box flow. If HUGOSTACK_ADMIN_PASS is
// set and config.json is missing or has an empty admin_pass, create/patch
// the config with the bcrypt-hashed password. A file that already holds a
// password is left alone — Settings is the change path after first run.
func bootstrapConfig() {
	pass := os.Getenv("HUGOSTACK_ADMIN_PASS")
	if pass == "" {
		return
	}
	p := config.Path()
	var c config.Config
	if data, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(data, &c); err != nil {
			log.Fatalf("bootstrap: %s is not valid JSON: %v", p, err)
		}
		if c.AdminPass != "" {
			return
		}
	} else {
		c.HugoPath = envOr("HUGOSTACK_HUGO_PATH", "/hugo")
		c.Port = envIntOr("HUGOSTACK_PORT", 8080)
		c.AdminUser = "admin"
	}
	if c.AdminUser == "" {
		c.AdminUser = "admin"
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	c.AdminPass = hash
	if err := config.Save(&c); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	log.Printf("bootstrap: wrote %s (admin_pass from HUGOSTACK_ADMIN_PASS, hugo_path %q, port %d)", p, c.HugoPath, c.Port)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envIntOr(k string, def int) int {
	v, err := strconv.Atoi(os.Getenv(k))
	if err != nil || v <= 0 {
		return def
	}
	return v
}
