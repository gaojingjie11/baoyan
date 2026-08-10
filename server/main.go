package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
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

var (
	db        *sql.DB
	jwtSecret string
	pepper    string
)

const (
	accessTTL  = 15 * time.Minute   // 短 token：每次请求携带
	refreshTTL = 7 * 24 * time.Hour // 长 token：1 周，用于换取新短 token
)

var (
	errUnauthorized = errors.New("unauthorized")
	errExpired      = errors.New("token expired")
)

// ---------- base64url ----------
func b64url(b []byte) string                { return base64.RawURLEncoding.EncodeToString(b) }
func b64urlDecode(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// ---------- JWT（HS256，手写，零外部依赖） ----------
func signJWT(claims map[string]interface{}, ttl time.Duration) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims["iat"] = now.Unix()
	claims["exp"] = now.Add(ttl).Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64url(header) + "." + b64url(payload)
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + b64url(mac.Sum(nil)), nil
}

func parseJWT(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errUnauthorized
	}
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := b64url(mac.Sum(nil))
	got, err := b64urlDecode(parts[2])
	if err != nil {
		return nil, errUnauthorized
	}
	if subtle.ConstantTimeCompare(got, []byte(expected)) != 1 {
		return nil, errUnauthorized
	}
	payload, err := b64urlDecode(parts[1])
	if err != nil {
		return nil, errUnauthorized
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errUnauthorized
	}
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, errExpired
		}
	}
	return claims, nil
}

// ---------- 密码哈希（stdlib：salt + sha256 + pepper，常量时间比较） ----------
func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	buf := make([]byte, 0, len(salt)+len(pw)+len(pepper))
	buf = append(buf, salt...)
	buf = append(buf, []byte(pw)...)
	buf = append(buf, []byte(pepper)...)
	sum := sha256.Sum256(buf)
	return b64url(salt) + ":" + b64url(sum[:]), nil
}

func verifyPassword(pw, stored string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := b64urlDecode(parts[0])
	if err != nil {
		return false
	}
	buf := make([]byte, 0, len(salt)+len(pw)+len(pepper))
	buf = append(buf, salt...)
	buf = append(buf, []byte(pw)...)
	buf = append(buf, []byte(pepper)...)
	sum := sha256.Sum256(buf)
	got := b64url(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(parts[1])) == 1
}

func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b64url(b), nil
}

// ---------- 鉴权辅助 ----------
func authUID(r *http.Request) (int, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return 0, errUnauthorized
	}
	claims, err := parseJWT(strings.TrimPrefix(auth, "Bearer "))
	if err != nil {
		return 0, err
	}
	uid, ok := claims["uid"].(float64)
	if !ok {
		return 0, errUnauthorized
	}
	return int(uid), nil
}

func writeJSON(w http.ResponseWriter, v interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

type userPublic struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}
type tokenResp struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	ExpiresIn    int        `json:"expires_in"`
	User         userPublic `json:"user"`
}

// 签发短 token + 生成长 token 并入库，返回完整登录响应
func issueAndRespond(w http.ResponseWriter, uid int, username string) {
	access, err := signJWT(map[string]interface{}{"uid": uid, "username": username}, accessTTL)
	if err != nil {
		writeJSON(w, map[string]string{"error": "sign error"}, http.StatusInternalServerError)
		return
	}
	rt, err := newRefreshToken()
	if err != nil {
		writeJSON(w, map[string]string{"error": "sign error"}, http.StatusInternalServerError)
		return
	}
	if _, err := db.Exec(`INSERT INTO refresh_tokens(token, user_id, expires_at) VALUES($1,$2,$3)`,
		rt, uid, time.Now().Add(refreshTTL)); err != nil {
		writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
		return
	}
	writeJSON(w, tokenResp{
		AccessToken:  access,
		RefreshToken: rt,
		ExpiresIn:    int(accessTTL.Seconds()),
		User:         userPublic{ID: uid, Username: username},
	}, http.StatusOK)
}

// ---------- 认证路由 ----------
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&c); err != nil {
		writeJSON(w, map[string]string{"error": "invalid body"}, http.StatusBadRequest)
		return
	}
	var id int
	var hash string
	err := db.QueryRow(`SELECT id, password_hash FROM users WHERE username=$1`, c.Username).Scan(&id, &hash)
	if err == sql.ErrNoRows || !verifyPassword(c.Password, hash) {
		writeJSON(w, map[string]string{"error": "用户名或密码错误"}, http.StatusUnauthorized)
		return
	} else if err != nil {
		writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
		return
	}
	issueAndRespond(w, id, c.Username)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&c); err != nil || c.Username == "" || c.Password == "" {
		writeJSON(w, map[string]string{"error": "用户名和密码必填"}, http.StatusBadRequest)
		return
	}
	hash, err := hashPassword(c.Password)
	if err != nil {
		writeJSON(w, map[string]string{"error": "error"}, http.StatusInternalServerError)
		return
	}
	var id int
	err = db.QueryRow(`INSERT INTO users(username, password_hash) VALUES($1,$2) ON CONFLICT(username) DO NOTHING RETURNING id`, c.Username, hash).Scan(&id)
	if err == sql.ErrNoRows {
		writeJSON(w, map[string]string{"error": "用户已存在"}, http.StatusConflict)
		return
	} else if err != nil {
		writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
		return
	}
	issueAndRespond(w, id, c.Username)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body)
	var uid int
	var username string
	err := db.QueryRow(`SELECT rt.user_id, u.username FROM refresh_tokens rt
		JOIN users u ON u.id=rt.user_id WHERE rt.token=$1 AND rt.expires_at > now()`,
		body.RefreshToken).Scan(&uid, &username)
	if err != nil {
		writeJSON(w, map[string]string{"error": "refresh token 无效或已过期"}, http.StatusUnauthorized)
		return
	}
	// 轮换：删旧发新，降低泄露风险
	db.Exec(`DELETE FROM refresh_tokens WHERE token=$1`, body.RefreshToken)
	issueAndRespond(w, uid, username)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body)
	if body.RefreshToken != "" {
		db.Exec(`DELETE FROM refresh_tokens WHERE token=$1`, body.RefreshToken)
	}
	writeJSON(w, map[string]bool{"ok": true}, http.StatusOK)
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := authUID(r)
	if err != nil {
		writeJSON(w, map[string]string{"error": "unauthorized"}, http.StatusUnauthorized)
		return
	}
	var username string
	db.QueryRow(`SELECT username FROM users WHERE id=$1`, uid).Scan(&username)
	writeJSON(w, userPublic{ID: uid, Username: username}, http.StatusOK)
}

// ---------- 进度路由（受保护，按用户隔离） ----------
var validStatus = map[string]bool{"": true, "applied": true, "iv": true, "adw": true, "adm": true}

func progressGetHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := authUID(r)
	if err != nil {
		writeJSON(w, map[string]string{"error": "unauthorized"}, http.StatusUnauthorized)
		return
	}
	rows, err := db.Query(`SELECT school_id, status FROM progress WHERE user_id=$1`, uid)
	if err != nil {
		writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var sid, st string
		rows.Scan(&sid, &st)
		m[sid] = st
	}
	writeJSON(w, m, http.StatusOK)
}

func progressPostHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := authUID(r)
	if err != nil {
		writeJSON(w, map[string]string{"error": "unauthorized"}, http.StatusUnauthorized)
		return
	}
	var m map[string]string
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&m); err != nil {
		writeJSON(w, map[string]string{"error": "invalid json"}, http.StatusBadRequest)
		return
	}
	for _, v := range m {
		if !validStatus[v] {
			writeJSON(w, map[string]string{"error": "invalid status value"}, http.StatusBadRequest)
			return
		}
	}
	tx, err := db.Begin()
	if err != nil {
		writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	for sid, st := range m {
		if _, err := tx.Exec(`INSERT INTO progress(user_id, school_id, status, updated_at)
			VALUES($1,$2,$3,now())
			ON CONFLICT (user_id, school_id) DO UPDATE SET status=EXCLUDED.status, updated_at=now()`,
			uid, sid, st); err != nil {
			writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, map[string]string{"error": "db error"}, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true}, http.StatusOK)
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func methodPost(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

// ---------- CORS ----------
func corsMiddleware(origins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimSpace(o)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed["*"] || allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func checkToken(r *http.Request, token string) bool {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	return h == "Bearer "+token || h == token
}

// ---------- 启动建库 / 建表 / 种子 ----------
func ensureDatabase(target string) {
	u, err := url.Parse(target)
	if err != nil {
		return
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return
	}
	b := *u
	b.Path = "/postgres"
	bdb, err := sql.Open("postgres", b.String())
	if err != nil {
		return
	}
	defer bdb.Close()
	if err = bdb.Ping(); err != nil {
		return
	}
	var exists bool
	if err = bdb.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", name).Scan(&exists); err != nil {
		return
	}
	if !exists {
		if _, err = bdb.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name)); err != nil {
			log.Printf("自动建库失败，请手动执行 CREATE DATABASE %s ：%v", name, err)
		} else {
			log.Printf("已自动创建数据库 %s", name)
		}
	}
}

func main() {
	port := getenv("PORT", "8080")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL 未设置")
	}
	cors := getenv("CORS_ORIGIN", "https://baoyan-one.vercel.app")

	jwtSecret = os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		jwtSecret = b64url(b)
		log.Println("警告：JWT_SECRET 未设置，已用随机密钥（重启后旧 token 失效）。生产请设置固定 JWT_SECRET。")
	}
	pepper = os.Getenv("PASSWORD_PEPPER")

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	db.SetMaxOpenConns(10)
	ensureDatabase(dbURL)

	for i := 0; i < 15; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("等待数据库就绪 (%d/15): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("数据库无法连接: %v", err)
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS progress (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			school_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (user_id, school_id)
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			token TEXT PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	}
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			log.Fatalf("建表失败: %v", err)
		}
	}

	// 种子用户：gao / gao040907
	var cnt int
	db.QueryRow(`SELECT count(*) FROM users WHERE username='gao'`).Scan(&cnt)
	if cnt == 0 {
		hash, herr := hashPassword("gao040907")
		if herr != nil {
			log.Fatalf("种子用户密码哈希失败: %v", herr)
		}
		if _, herr := db.Exec(`INSERT INTO users(username, password_hash) VALUES('gao',$1)`, hash); herr != nil {
			log.Fatalf("种子用户失败: %v", herr)
		}
		log.Println("已创建种子用户 gao / gao040907")
	}
	log.Println("数据库已就绪")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", health)
	mux.HandleFunc("/api/auth/register", methodPost(registerHandler))
	mux.HandleFunc("/api/auth/login", methodPost(loginHandler))
	mux.HandleFunc("/api/auth/refresh", methodPost(refreshHandler))
	mux.HandleFunc("/api/auth/logout", methodPost(logoutHandler))
	mux.HandleFunc("/api/me", meHandler)
	mux.HandleFunc("/api/progress", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			progressGetHandler(w, r)
		case http.MethodPost:
			progressPostHandler(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	handler := corsMiddleware(strings.Split(cors, ","), mux)
	addr := ":" + port
	log.Printf("后端监听 %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
