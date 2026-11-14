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
	"strings"

	"hugostack/internal/auth"
	"hugostack/internal/browser"
	"hugostack/internal/config"
)

var cfg *config.Config

var langFiles = map[string]string{"en": "index.md", "id": "index.id.md"}

func main() {
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
	pages["index"].ExecuteTemplate(w, "shell", data)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	data := newShellData("Settings", "Settings", "settings")
	data.BannerTitle = "Default admin password is \"admin\""
	data.BannerCopy = "Change it: run `go run ./cmd/setpass 'newpass'` and put the printed hash into config.json (admin_pass), then restart."
	data.HugoPath, data.Port, data.AdminUser = cfg.HugoPath, cfg.Port, cfg.AdminUser
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
	raw, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_, body, _ := browser.Split(string(raw))
		writeJSON(w, postJSON{
			Slug: slug, FileName: langFiles[lang], Lang: lang,
			Title: browser.TitleOf(string(raw)), Tags: browser.TagsOf(string(raw)),
			Description: fmDescription(string(raw)), Body: body,
		})
	case http.MethodPut:
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
		return err
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
	if err != nil {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	_, body, _ := browser.Split(string(raw))
	langList := []string{}
	for _, l := range []string{"en", "id"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), langFiles[l])); err == nil {
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
	data.PageJS = template.JS(fmt.Sprintf(editorJS, slug, lang))
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
