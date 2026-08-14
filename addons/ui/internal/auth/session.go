package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os/user"
	"sync"
	"time"
)

const (
	CookieName = "pqpm_ui_session"
	sessionTTL = 12 * time.Hour
)

var cookieMaxAge = int(sessionTTL.Seconds())


// Session holds an authenticated Linux user.
type Session struct {
	ID       string
	Username string
	UID      string
	GID      string
	HomeDir  string
	Expires  time.Time
}

// Store is an in-memory session store.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewStore() *Store {
	s := &Store{sessions: make(map[string]*Session)}
	go s.reap()
	return s
}

func (s *Store) reap() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		s.mu.Lock()
		now := time.Now()
		for id, sess := range s.sessions {
			if now.After(sess.Expires) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Store) Create(u *user.User) (*Session, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:       id,
		Username: u.Username,
		UID:      u.Uid,
		GID:      u.Gid,
		HomeDir:  u.HomeDir,
		Expires:  time.Now().Add(sessionTTL),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.Expires) {
		delete(s.sessions, id)
		return nil, false
	}
	return sess, true
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *Store) FromRequest(r *http.Request) (*Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	return s.Get(c.Value)
}

func SetCookie(w http.ResponseWriter, sess *Session, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   cookieMaxAge,
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
