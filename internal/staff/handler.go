package staff

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/model"
)

const tokenTTL = time.Hour

// dummyHash is compared when the user does not exist, so "unknown user" and "wrong password"
// take the same time and cannot be told apart.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

type Handler struct {
	DB        *gorm.DB
	JWTSecret []byte
}

type createRequest struct {
	Username string `json:"username" binding:"required,max=100"`
	Password string `json:"password" binding:"required,min=8,max=72"` // bcrypt only uses 72 bytes
	Hospital string `json:"hospital" binding:"required"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Hospital string `json:"hospital" binding:"required"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var hospital model.Hospital
	if err := h.DB.Where("code = ?", req.Hospital).First(&hospital).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown hospital"})
			return
		}
		internalError(c, err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(c, err)
		return
	}

	staff := model.Staff{HospitalID: hospital.ID, Username: req.Username, PasswordHash: string(hash)}
	// uniqueness is enforced by the (hospital_id, username) constraint, so two parallel requests can't both win
	if err := h.DB.Create(&staff).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			c.JSON(http.StatusConflict, gin.H{"error": "username already exists in this hospital"})
			return
		}
		internalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": staff.ID, "username": staff.Username, "hospital": hospital.Code})
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var staff model.Staff
	err := h.DB.Joins("JOIN hospitals ON hospitals.id = staffs.hospital_id").
		Where("hospitals.code = ? AND staffs.username = ?", req.Hospital, req.Username).
		First(&staff).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		internalError(c, err)
		return
	}

	hash := dummyHash
	if err == nil {
		hash = []byte(staff.PasswordHash)
	}
	// one message for every failure: don't reveal which of username / password / hospital was wrong
	if bcrypt.CompareHashAndPassword(hash, []byte(req.Password)) != nil || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username, password or hospital"})
		return
	}

	token, err := auth.IssueToken(h.JWTSecret, staff.ID, staff.HospitalID, tokenTTL)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(tokenTTL.Seconds()),
	})
}

func internalError(c *gin.Context, err error) {
	_ = c.Error(err) // shown by gin's logger; the client only sees a generic message
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
