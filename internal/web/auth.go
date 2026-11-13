package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/zhangchengzhen42-tech/Shop/internal/app"
)

type contextKey string

const userKey contextKey = "user"

func (s *Server) signedUser(id string) string {
	mac := hmac.New(sha256.New, []byte(s.cookieSecret))
	_, _ = mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString([]byte(id)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) verifyUser(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", false
	}
	id, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	if !hmac.Equal([]byte(s.signedUser(string(id))), []byte(value)) {
		return "", false
	}
	return string(id), true
}

func (s *Server) withUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("shop_session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		id, ok := s.verifyUser(cookie.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		user, err := s.store.User(r.Context(), id)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	}
}

func currentUser(r *http.Request) app.User {
	user, _ := r.Context().Value(userKey).(app.User)
	return user
}
