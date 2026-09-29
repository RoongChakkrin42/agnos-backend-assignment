package auth

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const hospitalIDKey = "hospital_id"

type Claims struct {
	HospitalID int64 `json:"hospital_id"`
	jwt.RegisteredClaims
}

func IssueToken(secret []byte, staffID, hospitalID int64, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		HospitalID: hospitalID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(staffID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

func ParseToken(secret []byte, token string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if claims.HospitalID == 0 {
		return nil, errors.New("token has no hospital")
	}
	return &claims, nil
}

// RequireStaff rejects requests without a valid bearer token and stores the staff's hospital in the context.
// The hospital used for searching comes only from here, never from the request body.
func RequireStaff(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := ParseToken(secret, raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(hospitalIDKey, claims.HospitalID)
		c.Next()
	}
}

// HospitalID returns the logged-in staff's hospital. Only valid behind RequireStaff.
func HospitalID(c *gin.Context) int64 { return c.GetInt64(hospitalIDKey) }
