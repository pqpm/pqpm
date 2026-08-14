package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/user"
	"strings"
	"time"

	"github.com/pqpm/pqpm/addons/ui/internal/auth"
	"github.com/pqpm/pqpm/addons/ui/internal/client"
	"github.com/pqpm/pqpm/addons/ui/internal/web"
)

// Config controls how the UI server authenticates and listens.
type Config struct {
	Addr     string // e.g. 127.0.0.1:9090
	PQPMPath string
	SelfPath string
	// AuthMode: "login" (Linux password via PAM/su) or "local" (current user, no password).
	AuthMode string
	// FixedUser forces a single username (local mode). Empty = current user.
	FixedUser string
	SecureCookie bool
}

// Server is the pqpm-ui HTTP server.
type Server struct {
	cfg    Config
	store  *auth.Store
	mux    *http.ServeMux
}

func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:9090"
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = "login"
	}
	if cfg.AuthMode != "login" && cfg.AuthMode != "local" {
		return nil, fmt.Errorf("invalid auth mode %q (use login or local)", cfg.AuthMode)
	}
	s := &Server{cfg: cfg, store: auth.NewStore(), mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/api/me", s.requireAuth(s.handleMe))
	s.mux.HandleFunc("/api/ping", s.requireAuth(s.handlePing))
	s.mux.HandleFunc("/api/status", s.requireAuth(s.handleStatus))
	s.mux.HandleFunc("/api/start", s.requireAuth(s.handleAction("start")))
	s.mux.HandleFunc("/api/stop", s.requireAuth(s.handleAction("stop")))
	s.mux.HandleFunc("/api/restart", s.requireAuth(s.handleAction("restart")))
	s.mux.HandleFunc("/api/reload", s.requireAuth(s.handleAction("reload")))
	s.mux.HandleFunc("/api/log", s.requireAuth(s.handleLog))
	s.mux.HandleFunc("/api/config", s.requireAuth(s.handleConfig))
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(web.StaticFS()))))
}

func (s *Server) clientFor(sess *auth.Session) *client.Client {
	return &client.Client{
		PQPM:     s.cfg.PQPMPath,
		Self:     s.cfg.SelfPath,
		Username: sess.Username,
	}
}

func (s *Server) requireAuth(next func(http.ResponseWriter, *http.Request, *auth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.session(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r, sess)
	}
}

func (s *Server) session(r *http.Request) (*auth.Session, error) {
	if s.cfg.AuthMode == "local" {
		u, err := s.localUser()
		if err != nil {
			return nil, err
		}
		return &auth.Session{
			Username: u.Username,
			UID:      u.Uid,
			GID:      u.Gid,
			HomeDir:  u.HomeDir,
		}, nil
	}
	sess, ok := s.store.FromRequest(r)
	if !ok {
		return nil, fmt.Errorf("no session")
	}
	return sess, nil
}

func (s *Server) localUser() (*user.User, error) {
	if s.cfg.FixedUser != "" {
		return user.Lookup(s.cfg.FixedUser)
	}
	return user.Current()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if s.cfg.AuthMode == "login" {
		if _, ok := s.store.FromRequest(r); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(web.IndexHTML())
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthMode == "local" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(web.LoginHTML(""))
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(web.LoginHTML("Invalid form"))
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		u, err := auth.AuthenticateUser(username, password)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(web.LoginHTML("Invalid username or password"))
			return
		}
		sess, err := s.store.Create(u)
		if err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
		auth.SetCookie(w, sess, s.cfg.SecureCookie)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		s.store.Delete(c.Value)
	}
	auth.ClearCookie(w)
	if s.cfg.AuthMode == "local" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"username": sess.Username,
		"home":     sess.HomeDir,
		"auth":     s.cfg.AuthMode,
	})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := s.clientFor(sess).Ping(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "pong"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rows, err := s.clientFor(sess).Status(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "services": rows})
}

func (s *Server) handleAction(action string) func(http.ResponseWriter, *http.Request, *auth.Session) {
	return func(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "POST required"})
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name required"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		cl := s.clientFor(sess)
		var msg string
		var err error
		switch action {
		case "start":
			msg, err = cl.Start(ctx, body.Name)
		case "stop":
			msg, err = cl.Stop(ctx, body.Name)
		case "restart":
			msg, err = cl.Restart(ctx, body.Name)
		case "reload":
			msg, err = cl.Reload(ctx, body.Name)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "unknown action"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error(), "message": msg})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
	}
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name required"})
		return
	}
	n := 100
	if v := r.URL.Query().Get("lines"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	text, err := s.clientFor(sess).LogLines(ctx, name, n)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "log": text})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	cl := s.clientFor(sess)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	switch r.Method {
	case http.MethodGet:
		content, path, err := cl.ReadConfig(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path, "content": content})
	case http.MethodPost, http.MethodPut:
		var body struct {
			Content string `json:"content"`
			Reload  bool   `json:"reload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid JSON"})
			return
		}
		if err := cl.WriteConfig(ctx, body.Content); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		msg := "config saved"
		if body.Reload {
			if m, err := cl.Reload(ctx, "*"); err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg, "reload_error": err.Error()})
				return
			} else if m != "" {
				msg = msg + "; " + m
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "GET or POST required"})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
