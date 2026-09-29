package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type Hospital struct {
	ID         int64
	Code       string
	Name       string
	APIBaseURL string `gorm:"column:api_base_url"`
	CreatedAt  time.Time
}

func (Hospital) TableName() string { return "hospitals" }

type Staff struct {
	ID           int64
	HospitalID   int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

func (Staff) TableName() string { return "staffs" }

// Patient mirrors the HIS patient response. Nullable columns are pointers so JSON shows null, not "".
type Patient struct {
	ID           int64     `json:"-"`
	HospitalID   int64     `json:"-"`
	PatientHN    string    `json:"patient_hn" gorm:"column:patient_hn"`
	NationalID   *string   `json:"national_id"`
	PassportID   *string   `json:"passport_id"`
	FirstNameTH  *string   `json:"first_name_th" gorm:"column:first_name_th"`
	MiddleNameTH *string   `json:"middle_name_th" gorm:"column:middle_name_th"`
	LastNameTH   *string   `json:"last_name_th" gorm:"column:last_name_th"`
	FirstNameEN  *string   `json:"first_name_en" gorm:"column:first_name_en"`
	MiddleNameEN *string   `json:"middle_name_en" gorm:"column:middle_name_en"`
	LastNameEN   *string   `json:"last_name_en" gorm:"column:last_name_en"`
	DateOfBirth  *Date     `json:"date_of_birth"`
	PhoneNumber  *string   `json:"phone_number"`
	Email        *string   `json:"email"`
	Gender       *string   `json:"gender"`
	CreatedAt    time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}

func (Patient) TableName() string { return "patients" }

const dateLayout = "2006-01-02"

// Date is a calendar date that reads and writes JSON as "YYYY-MM-DD" and maps to a Postgres DATE.
type Date struct{ time.Time }

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Format(dateLayout)) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("date must be YYYY-MM-DD: %w", err)
	}
	d.Time = t
	return nil
}

func (d *Date) Scan(v any) error {
	t, ok := v.(time.Time)
	if !ok {
		return fmt.Errorf("date: cannot scan %T", v)
	}
	d.Time = t
	return nil
}

func (d Date) Value() (driver.Value, error) { return d.Format(dateLayout), nil }
