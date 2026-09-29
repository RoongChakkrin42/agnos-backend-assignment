package staff_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/model"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/testutil"
)

type body = map[string]any

func TestCreateStaff_Success(t *testing.T) {
	r, db := testutil.NewServer(t)

	w := testutil.Do(r, http.MethodPost, "/staff/create",
		body{"username": "somchai", "password": "secret123", "hospital": "hospital-a"}, "")

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Hospital string `json:"hospital"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotZero(t, resp.ID)
	assert.Equal(t, "somchai", resp.Username)
	assert.Equal(t, "hospital-a", resp.Hospital)
	assert.NotContains(t, w.Body.String(), "password")

	var saved model.Staff
	require.NoError(t, db.First(&saved, resp.ID).Error)
	assert.Equal(t, testutil.HospitalID(t, db, "hospital-a"), saved.HospitalID)
	assert.NotEqual(t, "secret123", saved.PasswordHash, "password must not be stored as plain text")
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(saved.PasswordHash), []byte("secret123")))
}

func TestCreateStaff_SameUsernameInAnotherHospital(t *testing.T) {
	r, _ := testutil.NewServer(t)

	w := testutil.Do(r, http.MethodPost, "/staff/create", body{"username": "somchai", "password": "secret123", "hospital": "hospital-a"}, "")
	require.Equal(t, http.StatusCreated, w.Code)
	w = testutil.Do(r, http.MethodPost, "/staff/create", body{"username": "somchai", "password": "secret123", "hospital": "hospital-b"}, "")
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestCreateStaff_DuplicateUsername(t *testing.T) {
	r, _ := testutil.NewServer(t)
	req := body{"username": "somchai", "password": "secret123", "hospital": "hospital-a"}

	require.Equal(t, http.StatusCreated, testutil.Do(r, http.MethodPost, "/staff/create", req, "").Code)
	w := testutil.Do(r, http.MethodPost, "/staff/create", req, "")

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.JSONEq(t, `{"error":"username already exists in this hospital"}`, w.Body.String())
}

func TestCreateStaff_UnknownHospital(t *testing.T) {
	r, _ := testutil.NewServer(t)

	w := testutil.Do(r, http.MethodPost, "/staff/create",
		body{"username": "somchai", "password": "secret123", "hospital": "hospital-z"}, "")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"unknown hospital"}`, w.Body.String())
}

func TestCreateStaff_InvalidInput(t *testing.T) {
	r, _ := testutil.NewServer(t)

	cases := map[string]any{
		"missing username":   body{"password": "secret123", "hospital": "hospital-a"},
		"missing password":   body{"username": "somchai", "hospital": "hospital-a"},
		"missing hospital":   body{"username": "somchai", "password": "secret123"},
		"password too short": body{"username": "somchai", "password": "short", "hospital": "hospital-a"},
		"password too long":  body{"username": "somchai", "password": string(make([]byte, 73)), "hospital": "hospital-a"},
		"empty body":         nil,
		"broken json":        `{"username":`,
		"wrong type":         body{"username": 123, "password": "secret123", "hospital": "hospital-a"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			w := testutil.Do(r, http.MethodPost, "/staff/create", req, "")
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

func TestLogin_Success(t *testing.T) {
	r, db := testutil.NewServer(t)
	require.Equal(t, http.StatusCreated, testutil.Do(r, http.MethodPost, "/staff/create",
		body{"username": "somchai", "password": "secret123", "hospital": "hospital-b"}, "").Code)

	w := testutil.Do(r, http.MethodPost, "/staff/login",
		body{"username": "somchai", "password": "secret123", "hospital": "hospital-b"}, "")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, 3600, resp.ExpiresIn)

	claims, err := auth.ParseToken(testutil.JWTSecret, resp.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, testutil.HospitalID(t, db, "hospital-b"), claims.HospitalID, "token must carry the staff's hospital")
}

func TestLogin_WrongCredentials(t *testing.T) {
	r, _ := testutil.NewServer(t)
	require.Equal(t, http.StatusCreated, testutil.Do(r, http.MethodPost, "/staff/create",
		body{"username": "somchai", "password": "secret123", "hospital": "hospital-a"}, "").Code)

	cases := map[string]body{
		"wrong password":   {"username": "somchai", "password": "wrong-pass", "hospital": "hospital-a"},
		"unknown username": {"username": "nobody", "password": "secret123", "hospital": "hospital-a"},
		"other hospital":   {"username": "somchai", "password": "secret123", "hospital": "hospital-b"},
		"unknown hospital": {"username": "somchai", "password": "secret123", "hospital": "hospital-z"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			w := testutil.Do(r, http.MethodPost, "/staff/login", req, "")
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			// same message every time, so an attacker can't learn which part was wrong
			assert.JSONEq(t, `{"error":"invalid username, password or hospital"}`, w.Body.String())
		})
	}
}

func TestLogin_InvalidInput(t *testing.T) {
	r, _ := testutil.NewServer(t)

	cases := map[string]any{
		"missing username": body{"password": "secret123", "hospital": "hospital-a"},
		"missing password": body{"username": "somchai", "hospital": "hospital-a"},
		"missing hospital": body{"username": "somchai", "password": "secret123"},
		"empty body":       nil,
		"broken json":      `{`,
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			w := testutil.Do(r, http.MethodPost, "/staff/login", req, "")
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}
