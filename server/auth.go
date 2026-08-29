package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 7 * 24 * time.Hour
	argonTime  = 3
	argonMem   = 64 * 1024
	argonLane  = 2
	argonKey   = 32
)

var errUnauthorized = errors.New("unauthorized")
var errUserExists = errors.New("user exists")

type app struct {
	db            *sql.DB
	jwtSecret     []byte
	pepper        string
	secureCookies bool
}

type userPublic struct {
	ID       int             `json:"id"`
	Username string          `json:"username"`
	Theme    json.RawMessage `json:"theme"`
	Major    string          `json:"major"`
}

// 可选专业白名单。数据库中的空 major 一律视为 DefaultMajor（历史数据兼容）。
var majors = []string{"计算机", "地理信息科学", "工商管理", "工程管理"}

const defaultMajor = "计算机"

// normalizeMajor 把用户传入的专业归一化为合法值；空串表示「不改动」，非法值返回 ""。
func normalizeMajor(raw string) string {
	if raw == "" {
		return ""
	}
	for _, m := range majors {
		if m == raw {
			return m
		}
	}
	return ""
}

type session struct {
	AccessToken  string
	RefreshToken string
	RefreshUntil time.Time
	User         userPublic
}

type accessClaims struct {
	UID      int    `json:"uid"`
	Username string `json:"username"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func b64urlDecode(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b64url(b), nil
}

func passwordInput(password, pepper string) []byte {
	return []byte(password + "\x00" + pepper)
}

func hashPassword(password, pepper string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey(passwordInput(password, pepper), salt, argonTime, argonMem, argonLane, argonKey)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMem, argonTime, argonLane, b64url(salt), b64url(hash)), nil
}

func verifyPassword(password, encoded, pepper string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=2" {
		return false
	}
	salt, err := b64urlDecode(parts[4])
	if err != nil {
		return false
	}
	want, err := b64urlDecode(parts[5])
	if err != nil || len(want) != argonKey {
		return false
	}
	got := argon2.IDKey(passwordInput(password, pepper), salt, argonTime, argonMem, argonLane, argonKey)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (a *app) signAccess(user userPublic) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	now := time.Now()
	payload, err := json.Marshal(accessClaims{UID: user.ID, Username: user.Username, IssuedAt: now.Unix(), Expires: now.Add(accessTTL).Unix()})
	if err != nil {
		return "", err
	}
	input := b64url(header) + "." + b64url(payload)
	mac := hmac.New(sha256.New, a.jwtSecret)
	_, _ = mac.Write([]byte(input))
	return input + "." + b64url(mac.Sum(nil)), nil
}

func (a *app) authenticate(r *http.Request) (int, error) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return 0, errUnauthorized
	}
	parts := strings.Split(strings.TrimPrefix(auth, "Bearer "), ".")
	if len(parts) != 3 {
		return 0, errUnauthorized
	}
	mac := hmac.New(sha256.New, a.jwtSecret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := b64urlDecode(parts[2])
	if err != nil || subtle.ConstantTimeCompare(signature, mac.Sum(nil)) != 1 {
		return 0, errUnauthorized
	}
	var header map[string]string
	headerBytes, err := b64urlDecode(parts[0])
	if err != nil || json.Unmarshal(headerBytes, &header) != nil || header["alg"] != "HS256" || header["typ"] != "JWT" {
		return 0, errUnauthorized
	}
	var claims accessClaims
	payload, err := b64urlDecode(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil || claims.UID < 1 || claims.Expires <= time.Now().Unix() {
		return 0, errUnauthorized
	}
	return claims.UID, nil
}

func refreshDigest(raw string) [sha256.Size]byte { return sha256.Sum256([]byte(raw)) }

func (a *app) issueSession(ctx context.Context, tx *sql.Tx, user userPublic, familyID string) (session, error) {
	access, err := a.signAccess(user)
	if err != nil {
		return session{}, err
	}
	refresh, err := randomToken(32)
	if err != nil {
		return session{}, err
	}
	if familyID == "" {
		familyID, err = randomToken(16)
		if err != nil {
			return session{}, err
		}
	}
	expires := time.Now().Add(refreshTTL)
	digest := refreshDigest(refresh)
	if _, err := tx.ExecContext(ctx, `INSERT INTO refresh_tokens(token_hash, user_id, family_id, expires_at) VALUES($1,$2,$3,$4)`, digest[:], user.ID, familyID, expires); err != nil {
		return session{}, err
	}
	return session{AccessToken: access, RefreshToken: refresh, RefreshUntil: expires, User: user}, nil
}

// login 校验凭据。major 非空时把用户当前专业切换为该值；传空串表示沿用上次选择。
func (a *app) login(ctx context.Context, username, password, major string) (session, error) {
	var user userPublic
	var hash string
	err := a.db.QueryRowContext(ctx, `SELECT id, username, password_hash, theme_config, major FROM users WHERE username=$1`, username).Scan(&user.ID, &user.Username, &hash, &user.Theme, &user.Major)
	if errors.Is(err, sql.ErrNoRows) {
		return session{}, errUnauthorized
	}
	if err != nil {
		return session{}, err
	}
	if user.Major == "" {
		user.Major = defaultMajor
	}
	if !verifyPassword(password, hash, a.pepper) {
		return session{}, errUnauthorized
	}
	if major != "" && major != user.Major {
		if _, err := a.db.ExecContext(ctx, `UPDATE users SET major=$2 WHERE id=$1`, user.ID, major); err != nil {
			return session{}, err
		}
		user.Major = major
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return session{}, err
	}
	defer tx.Rollback()
	s, err := a.issueSession(ctx, tx, user, "")
	if err != nil {
		return session{}, err
	}
	if err := tx.Commit(); err != nil {
		return session{}, err
	}
	return s, nil
}

// register 创建新账号。密码由服务器用自身 pepper 做 Argon2id 哈希，调用方永远拿不到明文 pepper。
// major 非法时回退默认专业。用户名已存在返回 errUserExists。
func (a *app) register(ctx context.Context, username, password, major string) (userPublic, error) {
	username = strings.TrimSpace(username)
	if len(username) == 0 || len(username) > 64 || len(password) == 0 || len(password) > 256 {
		return userPublic{}, errors.New("invalid credentials")
	}
	var dummy int
	if err := a.db.QueryRowContext(ctx, `SELECT 1 FROM users WHERE username=$1`, username).Scan(&dummy); err == nil {
		return userPublic{}, errUserExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return userPublic{}, err
	}
	hash, err := hashPassword(password, a.pepper)
	if err != nil {
		return userPublic{}, err
	}
	mj := normalizeMajor(major)
	if mj == "" {
		mj = defaultMajor
	}
	var id int
	var theme json.RawMessage
	if err := a.db.QueryRowContext(ctx, `INSERT INTO users(username, password_hash, major) VALUES($1,$2,$3) RETURNING id, theme_config`, username, hash, mj).Scan(&id, &theme); err != nil {
		return userPublic{}, err
	}
	return userPublic{ID: id, Username: username, Theme: theme, Major: mj}, nil
}

func (a *app) refresh(ctx context.Context, raw string) (session, error) {
	if raw == "" {
		return session{}, errUnauthorized
	}
	digest := refreshDigest(raw)
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return session{}, err
	}
	defer tx.Rollback()
	var user userPublic
	var familyID string
	err = tx.QueryRowContext(ctx, `UPDATE refresh_tokens SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id, family_id`, digest[:]).Scan(&user.ID, &familyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var oldFamily string
			if lookupErr := tx.QueryRowContext(ctx, `SELECT family_id FROM refresh_tokens WHERE token_hash=$1`, digest[:]).Scan(&oldFamily); lookupErr == nil {
				if _, revokeErr := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=now() WHERE family_id=$1 AND revoked_at IS NULL`, oldFamily); revokeErr != nil {
					return session{}, revokeErr
				}
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return session{}, commitErr
			}
			return session{}, errUnauthorized
		}
		return session{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT username, theme_config, major FROM users WHERE id=$1`, user.ID).Scan(&user.Username, &user.Theme, &user.Major); err != nil {
		return session{}, err
	}
	if user.Major == "" {
		user.Major = defaultMajor
	}
	s, err := a.issueSession(ctx, tx, user, familyID)
	if err != nil {
		return session{}, err
	}
	if err := tx.Commit(); err != nil {
		return session{}, err
	}
	return s, nil
}

func (a *app) logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	digest := refreshDigest(raw)
	_, err := a.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL`, digest[:])
	return err
}
