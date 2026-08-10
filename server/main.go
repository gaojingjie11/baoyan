package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
)

var db *sql.DB

func main() {
	port := getenv("PORT", "8080")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL 未设置")
	}
	cors := getenv("CORS_ORIGIN", "https://baoyan-one.vercel.app")
	apiToken := os.Getenv("API_TOKEN") // 可选；留空则不校验

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	db.SetMaxOpenConns(10)
	ensureDatabase(dbURL)

	// 等待数据库就绪（docker-compose 里 db 可能稍慢）
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

	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS progress_store (
		id         TEXT PRIMARY KEY DEFAULT 'default',
		data       JSONB NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatalf("建表失败: %v", err)
	}
	log.Println("数据库已就绪")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", health)
	mux.HandleFunc("/api/progress", progressHandler(apiToken))

	handler := corsMiddleware(strings.Split(cors, ","), mux)
	addr := ":" + port
	log.Printf("后端监听 %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

// progressHandler 处理 GET（读取）/ POST（保存整份进度）/ OPTIONS（预检）
func progressHandler(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.Method {
		case http.MethodGet:
			getProgress(w)
		case http.MethodPost:
			if token != "" && !checkToken(r, token) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			postProgress(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getProgress(w http.ResponseWriter) {
	var raw []byte
	err := db.QueryRow(`SELECT data FROM progress_store WHERE id='default'`).Scan(&raw)
	if err == sql.ErrNoRows {
		raw = []byte("{}")
	} else if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func postProgress(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	valid := map[string]bool{"": true, "applied": true, "iv": true, "adw": true, "adm": true}
	for _, v := range m {
		if !valid[v] {
			http.Error(w, "invalid status value", http.StatusBadRequest)
			return
		}
	}
	b, _ := json.Marshal(m)
	_, err = db.Exec(`INSERT INTO progress_store (id, data, updated_at)
		VALUES ('default', $1, now())
		ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`, string(b))
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

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

// ensureDatabase 若目标库不存在，则连接到默认 postgres 库并自动创建，方便空 PG 直接跑
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
		return // 没有 postgres 默认库可引导，依赖目标库已存在
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

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
