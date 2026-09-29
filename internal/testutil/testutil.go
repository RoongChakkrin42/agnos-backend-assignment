// Package testutil gives each test its own empty Postgres database built from the real migrations/init.sql.
package testutil

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/database"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/his"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/model"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/server"
)

// Postgres from docker-compose.yml (host port 5434). Override with TEST_DATABASE_URL.
const defaultAdminURL = "postgres://agnos:agnos@localhost:5434/postgres?sslmode=disable"

var JWTSecret = []byte("test-secret-that-is-at-least-32-bytes")

// HISTimeout is short so timeout tests finish fast.
const HISTimeout = 300 * time.Millisecond

func init() { gin.SetMode(gin.TestMode) }

// NewDB creates a fresh database with the real schema and drops it when the test ends,
// so tests never share data and never touch the dev database.
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	admin, err := database.Open(adminURL)
	if err != nil {
		t.Fatalf("cannot reach test Postgres at %s (start it with `docker compose up -d db`): %v", adminURL, err)
	}

	name := "test_" + strings.ToLower(rand.Text())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, _ := url.Parse(adminURL)
	u.Path = "/" + name
	db, err := database.Open(u.String())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
		admin.Exec("DROP DATABASE " + name + " WITH (FORCE)")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})

	_, file, _, _ := runtime.Caller(0)
	schema, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../migrations/init.sql"))
	if err != nil {
		t.Fatalf("read init.sql: %v", err)
	}
	if err := db.Exec(string(schema)).Error; err != nil {
		t.Fatalf("apply init.sql: %v", err)
	}
	return db
}

// NewServer returns the real router backed by a fresh database.
func NewServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	db := NewDB(t)
	return server.New(db, JWTSecret, his.NewClient(HISTimeout)), db
}

// Do sends a JSON request. body may be a string (sent as-is) or any value (JSON-encoded). token may be "".
func Do(r http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	switch b := body.(type) {
	case nil:
	case string:
		buf.WriteString(b)
	default:
		json.NewEncoder(&buf).Encode(b)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func HospitalID(t *testing.T, db *gorm.DB, code string) int64 {
	t.Helper()
	var h model.Hospital
	if err := db.Where("code = ?", code).First(&h).Error; err != nil {
		t.Fatalf("hospital %q: %v", code, err)
	}
	return h.ID
}

// TokenFor issues a valid token for a staff member of the given hospital.
func TokenFor(t *testing.T, db *gorm.DB, hospitalCode string) string {
	t.Helper()
	token, err := auth.IssueToken(JWTSecret, 1, HospitalID(t, db, hospitalCode), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func Ptr[T any](v T) *T { return &v }
