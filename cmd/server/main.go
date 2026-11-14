package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"hugostack/internal/auth"
	"hugostack/internal/browser"
	"hugostack/internal/config"
)

var cfg *config.Config

func main() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./ui/static/"+r.URL.Path[len("/static/"):])
	})

	// Protected routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)

	http.Handle("/", auth.Middleware(mux))

	fmt.Printf("HugoStack starting on :%d (hugo: %s)\n", cfg.Port, cfg.HugoPath)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), nil))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		user := r.FormValue("user")
		pass := r.FormValue("pass")

		if user == cfg.AdminUser && auth.CheckPassword(cfg.AdminPass, pass) {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_id",
				Value:    auth.SessionToken(),
				Path:     "/",
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	fmt.Fprint(w, `<html><body><form method="POST">User: <input name="user"><br>Pass: <input name="pass" type="password"><br><button>Login</button></form></body></html>`)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	posts, err := browser.ListPosts(cfg.HugoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl := `<html>
	<head><link rel="stylesheet" href="/static/style.css"></head>
	<body>
		<header class="vq-header"><h1>HugoStack</h1></header>
		<main class="vq-container">
			<h2>Posts</h2>
			<ul class="vq-list">
				{{range .}}<li><a href="/edit/{{.Path}}">{{.Name}}</a></li>{{end}}
			</ul>
		</main>
	</body></html>`

	t := template.Must(template.New("index").Parse(tmpl))
	t.Execute(w, posts)
}
