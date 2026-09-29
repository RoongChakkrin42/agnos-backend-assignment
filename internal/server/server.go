package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/his"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/patient"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/staff"
)

func New(db *gorm.DB, jwtSecret []byte, hisClient *his.Client) *gin.Engine {
	staffHandler := &staff.Handler{DB: db, JWTSecret: jwtSecret}
	patientHandler := &patient.Handler{DB: db, HIS: hisClient}

	r := gin.Default()
	r.GET("/health", health(db))
	r.POST("/staff/create", staffHandler.Create)
	r.POST("/staff/login", staffHandler.Login)
	r.POST("/patient/search", auth.RequireStaff(jwtSecret), patientHandler.Search)
	return r
}

func health(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err == nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			err = sqlDB.PingContext(ctx)
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "db": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
