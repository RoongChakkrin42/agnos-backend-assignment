package patient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/his"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/model"
)

// ponytail: hard cap instead of pagination; add page/limit params when a hospital needs to browse more
const maxResults = 100

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

type Handler struct {
	DB  *gorm.DB
	HIS *his.Client
}

// searchRequest: every field is optional. Empty fields are ignored; filled fields are combined with AND.
type searchRequest struct {
	NationalID  string      `json:"national_id"`
	PassportID  string      `json:"passport_id"`
	FirstName   string      `json:"first_name"`
	MiddleName  string      `json:"middle_name"`
	LastName    string      `json:"last_name"`
	DateOfBirth *model.Date `json:"date_of_birth"`
	PhoneNumber string      `json:"phone_number"`
	Email       string      `json:"email"`
}

// Search looks in our database first. If nothing matches and the request has a national or passport ID,
// it asks the staff's hospital HIS, saves the patient, and searches again. That makes this service the
// middleware between staff and each HIS: HIS is called once per new patient, later searches are local.
func (h *Handler) Search(c *gin.Context) {
	var req searchRequest
	// an empty body means "no filters", same as {}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// hospital comes from the JWT only, never from the request body
	hospitalID := auth.HospitalID(c)

	patients, err := h.find(hospitalID, req)
	if err != nil {
		internalError(c, err)
		return
	}

	if id := req.hisID(); len(patients) == 0 && id != "" {
		err := h.importFromHIS(c.Request.Context(), hospitalID, id)
		switch {
		case errors.Is(err, his.ErrNotFound):
			// not in HIS either: an empty result is the right answer
		case errors.Is(err, his.ErrUnavailable):
			_ = c.Error(err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "hospital information system is unavailable, please try again"})
			return
		case err != nil:
			internalError(c, err)
			return
		default:
			// search again so the other filters in the request still apply to the imported patient
			if patients, err = h.find(hospitalID, req); err != nil {
				internalError(c, err)
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"patients": patients})
}

// hisID is the ID to ask HIS with. HIS only supports lookup by national ID or passport ID.
func (r searchRequest) hisID() string {
	if r.NationalID != "" {
		return r.NationalID
	}
	return r.PassportID
}

// find runs the search in our database. The hospital filter is part of the SQL, not applied after loading.
func (h *Handler) find(hospitalID int64, req searchRequest) ([]model.Patient, error) {
	q := h.DB.Where("hospital_id = ?", hospitalID)

	if req.NationalID != "" {
		q = q.Where("national_id = ?", req.NationalID)
	}
	if req.PassportID != "" {
		q = q.Where("passport_id = ?", req.PassportID)
	}
	// HIS stores names in Thai and English but the API has one field per name part: match either language,
	// case-insensitive, by prefix.
	for column, value := range map[string]string{
		"first_name":  req.FirstName,
		"middle_name": req.MiddleName,
		"last_name":   req.LastName,
	} {
		if value != "" {
			prefix := likeEscaper.Replace(value) + "%"
			q = q.Where("("+column+"_th ILIKE ? OR "+column+"_en ILIKE ?)", prefix, prefix)
		}
	}
	if req.DateOfBirth != nil {
		q = q.Where("date_of_birth = ?", req.DateOfBirth)
	}
	if req.PhoneNumber != "" {
		q = q.Where("phone_number = ?", req.PhoneNumber)
	}
	if req.Email != "" {
		q = q.Where("LOWER(email) = LOWER(?)", req.Email)
	}

	patients := []model.Patient{}
	err := q.Order("id").Limit(maxResults).Find(&patients).Error
	return patients, err
}

// importFromHIS fetches one patient from the hospital's HIS and inserts or updates it (keyed by hospital + HN).
func (h *Handler) importFromHIS(ctx context.Context, hospitalID int64, id string) error {
	var hospital model.Hospital
	if err := h.DB.First(&hospital, hospitalID).Error; err != nil {
		return err
	}
	p, err := h.HIS.SearchPatient(ctx, hospital.APIBaseURL, id)
	if err != nil {
		return err
	}
	p.HospitalID = hospital.ID
	return h.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hospital_id"}, {Name: "patient_hn"}},
		DoUpdates: clause.AssignmentColumns(hisColumns),
	}).Create(p).Error
}

// hisColumns are refreshed from HIS when the patient already exists.
var hisColumns = []string{
	"national_id", "passport_id",
	"first_name_th", "middle_name_th", "last_name_th",
	"first_name_en", "middle_name_en", "last_name_en",
	"date_of_birth", "phone_number", "email", "gender", "updated_at",
}

func internalError(c *gin.Context, err error) {
	_ = c.Error(err) // shown by gin's logger; the client only sees a generic message
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
