package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
)

var secret = []byte("test-secret-that-is-at-least-32-bytes")

func TestIssueAndParseToken(t *testing.T) {
	token, err := auth.IssueToken(secret, 7, 42, time.Hour)
	require.NoError(t, err)

	claims, err := auth.ParseToken(secret, token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.HospitalID)
	assert.Equal(t, "7", claims.Subject)
}

func TestParseTokenRejects(t *testing.T) {
	valid, _ := auth.IssueToken(secret, 1, 1, time.Hour)
	expired, _ := auth.IssueToken(secret, 1, 1, -time.Minute)
	noHospital, _ := auth.IssueToken(secret, 1, 0, time.Hour)
	otherAlg, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, auth.Claims{
		HospitalID:       1,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString(secret)
	noExpiry, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{HospitalID: 1}).SignedString(secret)

	cases := map[string]struct {
		secret []byte
		token  string
	}{
		"wrong secret":    {[]byte("another-secret-that-is-32-bytes-long"), valid},
		"expired":         {secret, expired},
		"no hospital":     {secret, noHospital},
		"other algorithm": {secret, otherAlg},
		"no expiry":       {secret, noExpiry},
		"garbage":         {secret, "not-a-jwt"},
		"tampered":        {secret, valid + "x"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := auth.ParseToken(tc.secret, tc.token)
			assert.Error(t, err)
		})
	}
}

func TestRequireStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/me", auth.RequireStaff(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"hospital_id": auth.HospitalID(c)})
	})
	valid, _ := auth.IssueToken(secret, 1, 42, time.Hour)

	cases := map[string]struct {
		header string
		status int
	}{
		"valid token":     {"Bearer " + valid, http.StatusOK},
		"no header":       {"", http.StatusUnauthorized},
		"not bearer":      {"Basic abc", http.StatusUnauthorized},
		"empty bearer":    {"Bearer ", http.StatusUnauthorized},
		"invalid token":   {"Bearer nope", http.StatusUnauthorized},
		"lowercase token": {"bearer " + valid, http.StatusUnauthorized},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				assert.JSONEq(t, `{"hospital_id":42}`, w.Body.String())
			}
		})
	}
}
