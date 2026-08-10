package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
)

var validStatus = map[string]bool{"": true, "applied": true, "iv": true, "adw": true, "adm": true, "failed": true}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func readJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func (a *app) setRefreshCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "baoyan_refresh", Value: token, Path: "/api/auth", Expires: expires, MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode})
}

func (a *app) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "baoyan_refresh", Value: "", Path: "/api/auth", MaxAge: -1, HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode})
}

func (a *app) loginHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil || len(body.Username) == 0 || len(body.Username) > 64 || len(body.Password) == 0 || len(body.Password) > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid credentials"})
		return
	}
	s, err := a.login(r.Context(), strings.TrimSpace(body.Username), body.Password)
	if errors.Is(err, errUnauthorized) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		log.Printf("login failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	a.setRefreshCookie(w, s.RefreshToken, s.RefreshUntil)
	writeJSON(w, http.StatusOK, map[string]any{"access_token": s.AccessToken, "expires_in": int(accessTTL.Seconds()), "user": s.User})
}

func (a *app) refreshHandler(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie("baoyan_refresh")
	raw := ""
	if cookie != nil {
		raw = cookie.Value
	}
	s, err := a.refresh(r.Context(), raw)
	if errors.Is(err, errUnauthorized) {
		a.clearRefreshCookie(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired"})
		return
	}
	if err != nil {
		log.Printf("refresh failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	a.setRefreshCookie(w, s.RefreshToken, s.RefreshUntil)
	writeJSON(w, http.StatusOK, map[string]any{"access_token": s.AccessToken, "expires_in": int(accessTTL.Seconds()), "user": s.User})
}

func (a *app) logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie("baoyan_refresh")
	if cookie != nil {
		if err := a.logout(r.Context(), cookie.Value); err != nil {
			log.Printf("logout failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
			return
		}
	}
	a.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) meHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var user userPublic
	err = a.db.QueryRowContext(r.Context(), `SELECT id, username, theme_config FROM users WHERE id=$1`, uid).Scan(&user.ID, &user.Username, &user.Theme)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *app) themeHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		Theme string `json:"theme"`
	}
	if err := readJSON(r, &body); err != nil || (body.Theme != "light" && body.Theme != "dark" && body.Theme != "system") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid theme"})
		return
	}
	theme, _ := json.Marshal(map[string]string{"theme": body.Theme})
	if _, err := a.db.ExecContext(r.Context(), `UPDATE users SET theme_config=$2 WHERE id=$1`, uid, theme); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"theme": body.Theme})
}

func (a *app) progressGetHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT school_id, status FROM progress WHERE user_id=$1`, uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	defer rows.Close()
	progress := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
			return
		}
		progress[id] = status
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

func validSchoolID(id string) bool {
	if len(id) == 0 || len(id) > 100 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func (a *app) progressPutHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	schoolID := r.PathValue("schoolID")
	var body struct {
		Status string `json:"status"`
	}
	if !validSchoolID(schoolID) || readJSON(r, &body) != nil || !validStatus[body.Status] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid progress"})
		return
	}
	if body.Status == "" {
		_, err = a.db.ExecContext(r.Context(), `DELETE FROM progress WHERE user_id=$1 AND school_id=$2`, uid, schoolID)
	} else {
		_, err = a.db.ExecContext(r.Context(), `INSERT INTO progress(user_id, school_id, status, updated_at) VALUES($1,$2,$3,now()) ON CONFLICT (user_id, school_id) DO UPDATE SET status=EXCLUDED.status, updated_at=now()`, uid, schoolID, body.Status)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func method(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		next(w, r)
	}
}

func (a *app) migrate(ctx context.Context) error {
	var legacyRefresh bool
	if err := a.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='refresh_tokens' AND column_name='token')`).Scan(&legacyRefresh); err != nil {
		return err
	}
	if legacyRefresh {
		// The prior table stored reusable raw session secrets. Sessions are intentionally invalidated.
		if _, err := a.db.ExecContext(ctx, `DROP TABLE refresh_tokens`); err != nil {
			return err
		}
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, theme_config JSONB NOT NULL DEFAULT '{"theme":"system"}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS theme_config JSONB NOT NULL DEFAULT '{"theme":"system"}'::jsonb`,
		`CREATE TABLE IF NOT EXISTS schools (id TEXT PRIMARY KEY, school TEXT NOT NULL, tier TEXT NOT NULL, college TEXT NOT NULL, direction TEXT NOT NULL, major TEXT NOT NULL, start_text TEXT NOT NULL, end_text TEXT NOT NULL, status TEXT NOT NULL, site TEXT NOT NULL, admit TEXT NOT NULL, source TEXT NOT NULL, remark TEXT NOT NULL, source_updated_at TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE IF NOT EXISTS progress (id SERIAL PRIMARY KEY, user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE, school_id TEXT NOT NULL, status TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(user_id, school_id))`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (token_hash BYTEA PRIMARY KEY, user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE, family_id TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE INDEX IF NOT EXISTS refresh_tokens_family_id_idx ON refresh_tokens(family_id)`,
	}
	for _, statement := range statements {
		if _, err := a.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) bootstrap(ctx context.Context) error {
	username, password := os.Getenv("BOOTSTRAP_USERNAME"), os.Getenv("BOOTSTRAP_PASSWORD")
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("bootstrap username and password must both be set")
	}
	hash, err := hashPassword(password, a.pepper)
	if err != nil {
		return err
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO users(username, password_hash) VALUES($1,$2)
		ON CONFLICT (username) DO UPDATE SET password_hash=EXCLUDED.password_hash
		WHERE users.password_hash NOT LIKE '$argon2id$%'`, username, hash)
	return err
}

func ensureDatabase(target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" || name == "postgres" {
		return nil
	}
	bootstrapURL := *u
	bootstrapURL.Path = "/postgres"
	bootstrapDB, err := sql.Open("postgres", bootstrapURL.String())
	if err != nil {
		return err
	}
	defer bootstrapDB.Close()
	if err := bootstrapDB.Ping(); err != nil {
		return err
	}
	var exists bool
	if err := bootstrapDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = bootstrapDB.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name))
	return err
}

func main() {
	dbURL, jwtSecret := os.Getenv("DATABASE_URL"), os.Getenv("JWT_SECRET")
	if dbURL == "" || len(jwtSecret) < 32 {
		log.Fatal("DATABASE_URL and JWT_SECRET (32+ bytes) are required")
	}
	if err := ensureDatabase(dbURL); err != nil {
		log.Fatalf("ensure database: %v", err)
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	if err := db.Ping(); err != nil {
		log.Fatalf("connect database: %v", err)
	}
	a := &app{db: db, jwtSecret: []byte(jwtSecret), pepper: os.Getenv("PASSWORD_PEPPER"), secureCookies: os.Getenv("COOKIE_SECURE") == "true"}
	ctx := context.Background()
	if err := a.migrate(ctx); err != nil {
		log.Fatalf("migrate database: %v", err)
	}
	if err := a.syncSchools(ctx); err != nil {
		log.Fatalf("sync schools: %v", err)
	}
	if err := a.ensureProgressSchoolForeignKey(ctx); err != nil {
		log.Fatalf("constrain progress: %v", err)
	}
	if err := a.bootstrap(ctx); err != nil {
		log.Fatalf("bootstrap user: %v", err)
	}
	if os.Getenv("MIGRATE_ONLY") == "1" {
		var schools, users, progress, sessions int
		if err := db.QueryRow(`SELECT (SELECT count(*) FROM schools), (SELECT count(*) FROM users), (SELECT count(*) FROM progress), (SELECT count(*) FROM refresh_tokens)`).Scan(&schools, &users, &progress, &sessions); err != nil {
			log.Fatalf("count migrated data: %v", err)
		}
		log.Printf("migration complete schools=%d users=%d progress=%d refresh_tokens=%d", schools, users, progress, sessions)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", method(http.MethodGet, func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }))
	mux.HandleFunc("/api/schools", method(http.MethodGet, a.schoolsHandler))
	mux.HandleFunc("/api/auth/login", method(http.MethodPost, a.loginHandler))
	mux.HandleFunc("/api/auth/refresh", method(http.MethodPost, a.refreshHandler))
	mux.HandleFunc("/api/auth/logout", method(http.MethodPost, a.logoutHandler))
	mux.HandleFunc("/api/me", method(http.MethodGet, a.meHandler))
	mux.HandleFunc("/api/me/theme", method(http.MethodPut, a.themeHandler))
	mux.HandleFunc("/api/progress", method(http.MethodGet, a.progressGetHandler))
	mux.HandleFunc("/api/progress/{schoolID}", method(http.MethodPut, a.progressPutHandler))
	server := &http.Server{Addr: ":" + getenv("PORT", "8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("api listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
