package auth

import (
	"crypto/rand"
	"net/http"
	"encoding/hex"
	"os"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

var (
	once         sync.Once
	sessionToken string
)

// SessionToken returns a stable per-process secret. Prefer HUGOSTACK_SESSION_SECRET;
// otherwise generate one at startup. Ponytail: in-memory only, resets on restart.
func SessionToken() string {
	once.Do(func() {
		if s := os.Getenv("HUGOSTACK_SESSION_SECRET"); s != "" {
			sessionToken = s
			return
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			sessionToken = "dev-insecure-token"
			return
		}
		sessionToken = hex.EncodeToString(b)
	})
	return sessionToken
}

func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(h, p string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(h), []byte(p))
	return err == nil
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err != nil || cookie.Value != SessionToken() {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
