package patient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/auth"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/model"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/testutil"
)

type body = map[string]any

var p = testutil.Ptr[string]

func date(s string) *model.Date {
	t, _ := time.Parse("2006-01-02", s)
	return &model.Date{Time: t}
}

// seed puts 3 patients in hospital A and 1 in hospital B.
// A-001 and B-001 share the same national ID on purpose (same person registered at two hospitals).
func seed(t *testing.T, db *gorm.DB) {
	a := testutil.HospitalID(t, db, "hospital-a")
	b := testutil.HospitalID(t, db, "hospital-b")
	patients := []model.Patient{
		{HospitalID: a, PatientHN: "A-001", NationalID: p("1100100100011"),
			FirstNameTH: p("สมชาย"), LastNameTH: p("ใจดี"), FirstNameEN: p("Somchai"), LastNameEN: p("Jaidee"),
			DateOfBirth: date("1990-05-12"), PhoneNumber: p("0811111111"), Email: p("Somchai@Example.com"), Gender: p("M")},
		{HospitalID: a, PatientHN: "A-002", PassportID: p("AA1234567"),
			FirstNameEN: p("John"), MiddleNameEN: p("Michael"), LastNameEN: p("Smith"),
			DateOfBirth: date("1985-01-30"), PhoneNumber: p("0822222222"), Email: p("john@example.com"), Gender: p("M")},
		{HospitalID: a, PatientHN: "A-003", NationalID: p("1100100100033"),
			FirstNameTH: p("สมหญิง"), LastNameTH: p("รักไทย"), FirstNameEN: p("Somying"), LastNameEN: p("Rakthai"),
			DateOfBirth: date("1990-05-12"), Gender: p("F")},
		{HospitalID: b, PatientHN: "B-001", NationalID: p("1100100100011"),
			FirstNameTH: p("สมชาย"), LastNameTH: p("ใจดี"), FirstNameEN: p("Somchai"), LastNameEN: p("Jaidee"),
			DateOfBirth: date("1990-05-12"), Gender: p("M")},
	}
	require.NoError(t, db.Create(&patients).Error)
}

// fakeHIS stands in for one hospital's HIS: it points that hospital's api_base_url at a local test server.
type fakeHIS struct{ calls atomic.Int32 }

func newFakeHIS(t *testing.T, db *gorm.DB, hospitalCode string, handler http.HandlerFunc) *fakeHIS {
	f := &fakeHIS{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, db.Model(&model.Hospital{}).Where("code = ?", hospitalCode).Update("api_base_url", srv.URL).Error)
	return f
}

func hisNotFound(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }

// hisReturns answers with body only for GET /patient/search/{id}; any other id is 404.
func hisReturns(id, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/patient/search/"+id {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

// setup: seeded database, and both hospitals' HIS answer "not found" unless a test replaces them.
func setup(t *testing.T) (*gin.Engine, *gorm.DB) {
	r, db := testutil.NewServer(t)
	seed(t, db)
	newFakeHIS(t, db, "hospital-a", hisNotFound)
	newFakeHIS(t, db, "hospital-b", hisNotFound)
	return r, db
}

func search(t *testing.T, r http.Handler, req any, token string) (int, []string, []map[string]any) {
	t.Helper()
	w := testutil.Do(r, http.MethodPost, "/patient/search", req, token)
	if w.Code != http.StatusOK {
		return w.Code, nil, nil
	}
	var resp struct {
		Patients []map[string]any `json:"patients"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Patients, "patients must be [] not null")
	hns := []string{}
	for _, pt := range resp.Patients {
		hns = append(hns, pt["patient_hn"].(string))
	}
	return w.Code, hns, resp.Patients
}

func TestSearch_Filters(t *testing.T) {
	r, db := setup(t)
	token := testutil.TokenFor(t, db, "hospital-a")

	cases := map[string]struct {
		req  any
		want []string
	}{
		"no filters returns all own patients": {body{}, []string{"A-001", "A-002", "A-003"}},
		"empty body same as no filters":       {nil, []string{"A-001", "A-002", "A-003"}},
		"national_id":                         {body{"national_id": "1100100100011"}, []string{"A-001"}},
		"passport_id":                         {body{"passport_id": "AA1234567"}, []string{"A-002"}},
		"first_name in Thai":                  {body{"first_name": "สมชาย"}, []string{"A-001"}},
		"first_name in English":               {body{"first_name": "Somchai"}, []string{"A-001"}},
		"first_name ignores case":             {body{"first_name": "SOMCHAI"}, []string{"A-001"}},
		"first_name prefix":                   {body{"first_name": "Som"}, []string{"A-001", "A-003"}},
		"first_name Thai prefix":              {body{"first_name": "สม"}, []string{"A-001", "A-003"}},
		"middle_name":                         {body{"middle_name": "Michael"}, []string{"A-002"}},
		"last_name in Thai":                   {body{"last_name": "รักไทย"}, []string{"A-003"}},
		"last_name in English":                {body{"last_name": "smith"}, []string{"A-002"}},
		"date_of_birth":                       {body{"date_of_birth": "1990-05-12"}, []string{"A-001", "A-003"}},
		"phone_number":                        {body{"phone_number": "0822222222"}, []string{"A-002"}},
		"email ignores case":                  {body{"email": "somchai@example.COM"}, []string{"A-001"}},
		"fields combine with AND":             {body{"first_name": "Som", "date_of_birth": "1990-05-12", "last_name": "Jai"}, []string{"A-001"}},
		"empty strings are ignored":           {body{"national_id": "", "first_name": ""}, []string{"A-001", "A-002", "A-003"}},
		"no match":                            {body{"national_id": "9999999999999"}, []string{}},
		"AND with conflict gives no match":    {body{"first_name": "Somchai", "date_of_birth": "2000-01-01"}, []string{}},
		"LIKE wildcard % is literal":          {body{"first_name": "%"}, []string{}},
		"LIKE wildcard _ is literal":          {body{"first_name": "_omchai"}, []string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, hns, _ := search(t, r, tc.req, token)
			require.Equal(t, http.StatusOK, code)
			assert.Equal(t, tc.want, hns)
		})
	}
}

func TestSearch_OnlyOwnHospital(t *testing.T) {
	r, db := setup(t)
	sharedID := body{"national_id": "1100100100011"}

	_, hns, _ := search(t, r, sharedID, testutil.TokenFor(t, db, "hospital-a"))
	assert.Equal(t, []string{"A-001"}, hns)

	_, hns, _ = search(t, r, sharedID, testutil.TokenFor(t, db, "hospital-b"))
	assert.Equal(t, []string{"B-001"}, hns)

	_, hns, _ = search(t, r, body{}, testutil.TokenFor(t, db, "hospital-b"))
	assert.Equal(t, []string{"B-001"}, hns, "no filters must still stay inside the staff's hospital")
}

func TestSearch_HospitalInBodyIsIgnored(t *testing.T) {
	r, db := setup(t)
	bID := testutil.HospitalID(t, db, "hospital-b")

	_, hns, _ := search(t, r, body{"hospital_id": bID, "hospital": "hospital-b"}, testutil.TokenFor(t, db, "hospital-a"))

	assert.Equal(t, []string{"A-001", "A-002", "A-003"}, hns)
}

func TestSearch_ResponseShape(t *testing.T) {
	r, db := setup(t)

	_, _, patients := search(t, r, body{"national_id": "1100100100011"}, testutil.TokenFor(t, db, "hospital-a"))

	require.Len(t, patients, 1)
	got := patients[0]
	assert.Equal(t, "A-001", got["patient_hn"])
	assert.Equal(t, "สมชาย", got["first_name_th"])
	assert.Equal(t, "Somchai", got["first_name_en"])
	assert.Equal(t, "1990-05-12", got["date_of_birth"])
	assert.Equal(t, "M", got["gender"])
	assert.Nil(t, got["passport_id"])
	assert.NotContains(t, got, "hospital_id", "internal ids must not leak")
	assert.NotContains(t, got, "id")
}

func TestSearch_Unauthorized(t *testing.T) {
	r, db := setup(t)
	expired, _ := auth.IssueToken(testutil.JWTSecret, 1, testutil.HospitalID(t, db, "hospital-a"), -time.Minute)
	forged, _ := auth.IssueToken([]byte("attacker-secret-that-is-32-bytes!!"), 1, testutil.HospitalID(t, db, "hospital-a"), time.Hour)

	cases := map[string]string{
		"no token":      "",
		"garbage token": "not-a-jwt",
		"expired token": expired,
		"forged token":  forged,
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			w := testutil.Do(r, http.MethodPost, "/patient/search", body{}, token)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.NotContains(t, w.Body.String(), "patient_hn")
		})
	}
}

func TestSearch_InvalidInput(t *testing.T) {
	r, db := setup(t)
	token := testutil.TokenFor(t, db, "hospital-a")

	cases := map[string]any{
		"bad date format": body{"date_of_birth": "12/05/1990"},
		"impossible date": body{"date_of_birth": "1990-13-40"},
		"date not string": body{"date_of_birth": 19900512},
		"broken json":     `{"national_id":`,
		"wrong type":      body{"national_id": 1100100100011},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			w := testutil.Do(r, http.MethodPost, "/patient/search", req, token)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

func TestSearch_EndToEndWithLogin(t *testing.T) {
	r, _ := setup(t)
	require.Equal(t, http.StatusCreated, testutil.Do(r, http.MethodPost, "/staff/create",
		body{"username": "nurse", "password": "secret123", "hospital": "hospital-b"}, "").Code)
	w := testutil.Do(r, http.MethodPost, "/staff/login",
		body{"username": "nurse", "password": "secret123", "hospital": "hospital-b"}, "")
	require.Equal(t, http.StatusOK, w.Code)
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &login))

	_, hns, _ := search(t, r, body{"first_name": "สมชาย"}, login.AccessToken)

	assert.Equal(t, []string{"B-001"}, hns)
}

const hisPatient = `{
	"first_name_th": "มานี", "middle_name_th": "", "last_name_th": "มีนา",
	"first_name_en": "Manee", "middle_name_en": "", "last_name_en": "Meena",
	"date_of_birth": "1995-03-01", "patient_hn": "HN-900",
	"national_id": "3333333333333", "passport_id": "",
	"phone_number": "0899999999", "email": "manee@example.com", "gender": "F"
}`

func countPatients(t *testing.T, db *gorm.DB, hospitalCode string) int64 {
	var n int64
	require.NoError(t, db.Model(&model.Patient{}).Where("hospital_id = ?", testutil.HospitalID(t, db, hospitalCode)).Count(&n).Error)
	return n
}

func TestSearch_HIS_ImportsNewPatient(t *testing.T) {
	r, db := setup(t)
	hisA := newFakeHIS(t, db, "hospital-a", hisReturns("3333333333333", hisPatient))
	token := testutil.TokenFor(t, db, "hospital-a")

	code, hns, patients := search(t, r, body{"national_id": "3333333333333"}, token)

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []string{"HN-900"}, hns)
	assert.Equal(t, "มานี", patients[0]["first_name_th"])
	assert.Equal(t, "Meena", patients[0]["last_name_en"])
	assert.Equal(t, "1995-03-01", patients[0]["date_of_birth"])
	assert.Equal(t, "F", patients[0]["gender"])
	assert.Nil(t, patients[0]["passport_id"])
	assert.EqualValues(t, 1, hisA.calls.Load())
	assert.EqualValues(t, 4, countPatients(t, db, "hospital-a"), "patient is saved in our database")

	// second time it is found locally: HIS is not called again
	_, hns, _ = search(t, r, body{"national_id": "3333333333333"}, token)
	assert.Equal(t, []string{"HN-900"}, hns)
	assert.EqualValues(t, 1, hisA.calls.Load())

	// and it can now be found by name too
	_, hns, _ = search(t, r, body{"first_name": "มานี"}, token)
	assert.Equal(t, []string{"HN-900"}, hns)
}

func TestSearch_HIS_ByPassportID(t *testing.T) {
	r, db := setup(t)
	newFakeHIS(t, db, "hospital-a", hisReturns("ZZ7654321", `{"patient_hn":"HN-901","passport_id":"ZZ7654321","first_name_en":"Anna"}`))

	_, hns, _ := search(t, r, body{"passport_id": "ZZ7654321"}, testutil.TokenFor(t, db, "hospital-a"))

	assert.Equal(t, []string{"HN-901"}, hns)
}

func TestSearch_HIS_UsesOnlyStaffHospital(t *testing.T) {
	r, db := setup(t)
	hisA := newFakeHIS(t, db, "hospital-a", hisNotFound)
	hisB := newFakeHIS(t, db, "hospital-b", hisReturns("3333333333333", hisPatient))

	_, hns, _ := search(t, r, body{"national_id": "3333333333333"}, testutil.TokenFor(t, db, "hospital-b"))
	assert.Equal(t, []string{"HN-900"}, hns)
	assert.EqualValues(t, 1, hisB.calls.Load())
	assert.EqualValues(t, 0, hisA.calls.Load(), "hospital B staff must never trigger hospital A's HIS")
	assert.EqualValues(t, 2, countPatients(t, db, "hospital-b"))
	assert.EqualValues(t, 3, countPatients(t, db, "hospital-a"))

	// the patient imported for B is not visible to A; A asks its own HIS, which doesn't know them
	_, hns, _ = search(t, r, body{"national_id": "3333333333333"}, testutil.TokenFor(t, db, "hospital-a"))
	assert.Equal(t, []string{}, hns)
	assert.EqualValues(t, 1, hisA.calls.Load())
}

func TestSearch_HIS_NotCalled(t *testing.T) {
	r, db := setup(t)
	hisA := newFakeHIS(t, db, "hospital-a", hisReturns("3333333333333", hisPatient))
	token := testutil.TokenFor(t, db, "hospital-a")

	cases := map[string]body{
		"found in database":       {"national_id": "1100100100011"},
		"no id, only name":        {"first_name": "Manee"},
		"no id, nothing found":    {"last_name": "Nobody", "date_of_birth": "2001-01-01"},
		"no filters":              {},
		"found by passport local": {"passport_id": "AA1234567"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			code, _, _ := search(t, r, req, token)
			assert.Equal(t, http.StatusOK, code)
		})
	}
	assert.EqualValues(t, 0, hisA.calls.Load())
}

func TestSearch_HIS_NotFound(t *testing.T) {
	r, db := setup(t)
	hisA := newFakeHIS(t, db, "hospital-a", hisNotFound)

	code, hns, _ := search(t, r, body{"national_id": "9999999999999"}, testutil.TokenFor(t, db, "hospital-a"))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, []string{}, hns)
	assert.EqualValues(t, 1, hisA.calls.Load())
}

func TestSearch_HIS_Unavailable(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"server error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"bad body":     func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>")) },
		"timeout": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(5 * testutil.HISTimeout):
			case <-r.Context().Done():
			}
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			r, db := setup(t)
			newFakeHIS(t, db, "hospital-a", handler)

			w := testutil.Do(r, http.MethodPost, "/patient/search", body{"national_id": "3333333333333"}, testutil.TokenFor(t, db, "hospital-a"))

			// "HIS is broken" must look different from "patient does not exist"
			assert.Equal(t, http.StatusBadGateway, w.Code)
			assert.JSONEq(t, `{"error":"hospital information system is unavailable, please try again"}`, w.Body.String())
			assert.EqualValues(t, 3, countPatients(t, db, "hospital-a"), "nothing is saved")
		})
	}
}

func TestSearch_HIS_OtherFiltersStillApply(t *testing.T) {
	r, db := setup(t)
	newFakeHIS(t, db, "hospital-a", hisReturns("3333333333333", hisPatient))

	// HIS finds the ID, but the birth date in the request doesn't match that patient
	_, hns, _ := search(t, r, body{"national_id": "3333333333333", "date_of_birth": "2000-01-01"}, testutil.TokenFor(t, db, "hospital-a"))

	assert.Equal(t, []string{}, hns)
}

func TestSearch_HIS_UpdatesExistingPatient(t *testing.T) {
	r, db := setup(t)
	// HIS says A-002 (already in our DB, found by passport before) now also has a national ID and a Thai name
	newFakeHIS(t, db, "hospital-a", hisReturns("4444444444444",
		`{"patient_hn":"A-002","national_id":"4444444444444","passport_id":"AA1234567","first_name_th":"จอห์น","first_name_en":"John","last_name_en":"Smith","gender":"M"}`))

	_, hns, patients := search(t, r, body{"national_id": "4444444444444"}, testutil.TokenFor(t, db, "hospital-a"))

	assert.Equal(t, []string{"A-002"}, hns)
	assert.Equal(t, "จอห์น", patients[0]["first_name_th"])
	assert.EqualValues(t, 3, countPatients(t, db, "hospital-a"), "same HN is updated, not duplicated")
}
