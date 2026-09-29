// Package his calls a hospital's HIS (Hospital Information System) patient API.
package his

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/model"
)

var (
	// ErrNotFound: HIS answered normally and has no such patient. Retrying won't help.
	ErrNotFound = errors.New("his: patient not found")
	// ErrUnavailable: HIS is down, slow, or answered with something we can't use. Retrying later may help.
	ErrUnavailable = errors.New("his: unavailable")
)

const maxBodyBytes = 1 << 20

type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{http: &http.Client{Timeout: timeout}}
}

// patientResponse is the body of GET /patient/search/{id}.
type patientResponse struct {
	FirstNameTH  string `json:"first_name_th"`
	MiddleNameTH string `json:"middle_name_th"`
	LastNameTH   string `json:"last_name_th"`
	FirstNameEN  string `json:"first_name_en"`
	MiddleNameEN string `json:"middle_name_en"`
	LastNameEN   string `json:"last_name_en"`
	DateOfBirth  string `json:"date_of_birth"`
	PatientHN    string `json:"patient_hn"`
	NationalID   string `json:"national_id"`
	PassportID   string `json:"passport_id"`
	PhoneNumber  string `json:"phone_number"`
	Email        string `json:"email"`
	Gender       string `json:"gender"`
}

// SearchPatient calls GET {baseURL}/patient/search/{id}, where id is a national ID or passport ID.
// It returns ErrNotFound for 404 and an error wrapping ErrUnavailable for every other failure.
func (c *Client) SearchPatient(ctx context.Context, baseURL, id string) (*model.Patient, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/patient/search/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	var body patientResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: decode body: %v", ErrUnavailable, err)
	}
	p, err := body.toModel()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return p, nil
}

func (r patientResponse) toModel() (*model.Patient, error) {
	if r.PatientHN == "" {
		return nil, errors.New("response has no patient_hn")
	}
	p := &model.Patient{
		PatientHN:    r.PatientHN,
		NationalID:   optional(r.NationalID),
		PassportID:   optional(r.PassportID),
		FirstNameTH:  optional(r.FirstNameTH),
		MiddleNameTH: optional(r.MiddleNameTH),
		LastNameTH:   optional(r.LastNameTH),
		FirstNameEN:  optional(r.FirstNameEN),
		MiddleNameEN: optional(r.MiddleNameEN),
		LastNameEN:   optional(r.LastNameEN),
		PhoneNumber:  optional(r.PhoneNumber),
		Email:        optional(r.Email),
	}
	if r.DateOfBirth != "" {
		t, err := time.Parse("2006-01-02", r.DateOfBirth)
		if err != nil {
			return nil, fmt.Errorf("bad date_of_birth %q", r.DateOfBirth)
		}
		p.DateOfBirth = &model.Date{Time: t}
	}
	// the spec only allows M or F; anything else is stored as unknown rather than failing the whole patient
	if r.Gender == "M" || r.Gender == "F" {
		p.Gender = &r.Gender
	}
	return p, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
