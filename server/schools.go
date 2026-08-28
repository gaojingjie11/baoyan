package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

type schoolID string

func (id *schoolID) UnmarshalJSON(data []byte) error {
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		*id = schoolID(asString)
		return nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(data, &asNumber); err != nil {
		return fmt.Errorf("school id: %w", err)
	}
	*id = schoolID(asNumber.String())
	return nil
}

type school struct {
	ID        schoolID `json:"id"`
	School    string   `json:"school"`
	Type      string   `json:"type"`
	College   string   `json:"college"`
	Direction string   `json:"direction"`
	Major     string   `json:"major"`
	Start     string   `json:"start"`
	End       string   `json:"end"`
	Status    string   `json:"status"`
	Site      string   `json:"site"`
	Admit     string   `json:"admit"`
	Source    string   `json:"source"`
	Remark    string   `json:"remark"`
}

type schoolCatalog struct {
	UpdatedAt string   `json:"updated_at"`
	Schools   []school `json:"schools"`
}

func decodeSchools(reader io.Reader) (schoolCatalog, error) {
	decoder := json.NewDecoder(reader)
	var catalog schoolCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return schoolCatalog{}, err
	}
	if len(catalog.Schools) == 0 {
		return schoolCatalog{}, fmt.Errorf("school catalogue is empty")
	}
	seen := make(map[schoolID]struct{}, len(catalog.Schools))
	for _, item := range catalog.Schools {
		if item.ID == "" || item.School == "" {
			return schoolCatalog{}, fmt.Errorf("school id and name are required")
		}
		if _, exists := seen[item.ID]; exists {
			return schoolCatalog{}, fmt.Errorf("duplicate school id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return catalog, nil
}

func loadSchoolCatalog(path string) (schoolCatalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return schoolCatalog{}, err
	}
	defer file.Close()
	return decodeSchools(file)
}

func (a *app) syncSchools(ctx context.Context) error {
	path := getenv("SCHOOLS_FILE", "/app/schools.json")
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		path = "../schools.json"
	}
	catalog, err := loadSchoolCatalog(path)
	if err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const upsert = `INSERT INTO schools (id, school, tier, college, direction, major, start_text, end_text, status, site, admit, source, remark, source_updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET school=EXCLUDED.school, tier=EXCLUDED.tier, college=EXCLUDED.college,
		direction=EXCLUDED.direction, major=EXCLUDED.major, start_text=EXCLUDED.start_text, end_text=EXCLUDED.end_text,
		status=EXCLUDED.status, site=EXCLUDED.site, admit=EXCLUDED.admit, source=EXCLUDED.source,
		remark=EXCLUDED.remark, source_updated_at=EXCLUDED.source_updated_at, updated_at=now()`
	for _, item := range catalog.Schools {
		if _, err := tx.ExecContext(ctx, upsert, string(item.ID), item.School, item.Type, item.College, item.Direction, item.Major, item.Start, item.End, item.Status, item.Site, item.Admit, item.Source, item.Remark, catalog.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *app) ensureProgressSchoolForeignKey(ctx context.Context) error {
	if _, err := a.db.ExecContext(ctx, `DELETE FROM progress WHERE NOT EXISTS (SELECT 1 FROM schools WHERE schools.id=progress.school_id)`); err != nil {
		return err
	}
	_, err := a.db.ExecContext(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='progress_school_id_fkey') THEN
			ALTER TABLE progress ADD CONSTRAINT progress_school_id_fkey FOREIGN KEY (school_id) REFERENCES schools(id);
		END IF;
	END $$`)
	return err
}

// majorExpr 把 schools 表里的空 major 归一为默认专业，兼容加专业之前的历史数据。
// 字面量必须与 auth.go 的 defaultMajor 保持一致。
const majorExpr = `COALESCE(NULLIF(major,''), '计算机')`

func (a *app) schoolsHandler(w http.ResponseWriter, r *http.Request) {
	major := normalizeMajor(r.URL.Query().Get("major"))
	query := `SELECT id, school, tier, college, direction, ` + majorExpr + `, start_text, end_text, status, site, admit, source, remark FROM schools`
	updatedQuery := `SELECT COALESCE(MAX(source_updated_at), '') FROM schools`
	var args []any
	if major != "" {
		query += ` WHERE ` + majorExpr + ` = $1`
		updatedQuery += ` WHERE ` + majorExpr + ` = $1`
		args = append(args, major)
	}
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "server error"})
		return
	}
	defer rows.Close()
	var schools []school
	for rows.Next() {
		var item school
		var id string
		if err := rows.Scan(&id, &item.School, &item.Type, &item.College, &item.Direction, &item.Major, &item.Start, &item.End, &item.Status, &item.Site, &item.Admit, &item.Source, &item.Remark); err != nil {
			writeJSON(w, 500, map[string]string{"error": "server error"})
			return
		}
		item.ID = schoolID(id)
		schools = append(schools, item)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, 500, map[string]string{"error": "server error"})
		return
	}
	sort.Slice(schools, func(i, j int) bool {
		left, right := schools[i], schools[j]
		if left.Type != right.Type {
			return left.Type == "985"
		}
		li, _ := strconv.Atoi(string(left.ID))
		ri, _ := strconv.Atoi(string(right.ID))
		return li < ri
	})
	var updatedAt string
	if err := a.db.QueryRowContext(r.Context(), updatedQuery, args...).Scan(&updatedAt); err != nil {
		writeJSON(w, 500, map[string]string{"error": "server error"})
		return
	}
	writeJSON(w, 200, schoolCatalog{UpdatedAt: strings.TrimSpace(updatedAt), Schools: schools})
}
