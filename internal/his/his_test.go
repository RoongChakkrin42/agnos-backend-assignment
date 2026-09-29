package his_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/his"
)

const fullPatient = `{
	"first_name_th": "สมชาย", "middle_name_th": "", "last_name_th": "ใจดี",
	"first_name_en": "Somchai", "middle_name_en": "", "last_name_en": "Jaidee",
	"date_of_birth": "1990-05-12", "patient_hn": "HN-001",
	"national_id": "1100100100011", "passport_id": "",
	"phone_number": "0811111111", "email": "somchai@example.com", "gender": "M"
}`

func fakeHIS(t *testing.T, status int, body string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchPatient_Found(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Write([]byte(fullPatient))
	}))
	defer srv.Close()

	p, err := his.NewClient(time.Second).SearchPatient(context.Background(), srv.URL+"/", "1100100100011")

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/patient/search/1100100100011", gotPath)
	assert.Equal(t, "HN-001", p.PatientHN)
	assert.Equal(t, "สมชาย", *p.FirstNameTH)
	assert.Equal(t, "ใจดี", *p.LastNameTH)
	assert.Equal(t, "Somchai", *p.FirstNameEN)
	assert.Equal(t, "Jaidee", *p.LastNameEN)
	assert.Equal(t, "1100100100011", *p.NationalID)
	assert.Equal(t, "0811111111", *p.PhoneNumber)
	assert.Equal(t, "somchai@example.com", *p.Email)
	assert.Equal(t, "M", *p.Gender)
	assert.Equal(t, "1990-05-12", p.DateOfBirth.Format("2006-01-02"))
	assert.Nil(t, p.PassportID, "empty strings become NULL")
	assert.Nil(t, p.MiddleNameTH)
}

func TestSearchPatient_UnknownGenderIsDropped(t *testing.T) {
	srv := fakeHIS(t, http.StatusOK, `{"patient_hn":"HN-1","gender":"X"}`)

	p, err := his.NewClient(time.Second).SearchPatient(context.Background(), srv.URL, "1")

	require.NoError(t, err)
	assert.Nil(t, p.Gender)
	assert.Nil(t, p.DateOfBirth)
}

func TestSearchPatient_IDIsPathEscaped(t *testing.T) {
	var rawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := his.NewClient(time.Second).SearchPatient(context.Background(), srv.URL, "../admin?x=1")

	assert.ErrorIs(t, err, his.ErrNotFound)
	assert.Equal(t, "/patient/search/..%2Fadmin%3Fx=1", rawPath, "id must not change the path or add a query")
}

func TestSearchPatient_NotFound(t *testing.T) {
	srv := fakeHIS(t, http.StatusNotFound, `{"error":"not found"}`)

	_, err := his.NewClient(time.Second).SearchPatient(context.Background(), srv.URL, "1")

	assert.ErrorIs(t, err, his.ErrNotFound)
	assert.NotErrorIs(t, err, his.ErrUnavailable)
}

func TestSearchPatient_Unavailable(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"server error":    {http.StatusInternalServerError, ``},
		"bad gateway":     {http.StatusBadGateway, ``},
		"bad request":     {http.StatusBadRequest, ``},
		"broken json":     {http.StatusOK, `{"patient_hn":`},
		"not json":        {http.StatusOK, `<html>maintenance</html>`},
		"no patient_hn":   {http.StatusOK, `{"first_name_en":"Somchai"}`},
		"bad birth date":  {http.StatusOK, `{"patient_hn":"HN-1","date_of_birth":"12/05/1990"}`},
		"wrong json type": {http.StatusOK, `{"patient_hn":123}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fakeHIS(t, tc.status, tc.body)

			_, err := his.NewClient(time.Second).SearchPatient(context.Background(), srv.URL, "1")

			assert.ErrorIs(t, err, his.ErrUnavailable)
			assert.NotErrorIs(t, err, his.ErrNotFound)
		})
	}
}

func TestSearchPatient_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	start := time.Now()
	_, err := his.NewClient(50*time.Millisecond).SearchPatient(context.Background(), srv.URL, "1")

	assert.ErrorIs(t, err, his.ErrUnavailable)
	assert.Less(t, time.Since(start), time.Second, "must give up at the timeout, not wait for HIS")
}

func TestSearchPatient_ServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := his.NewClient(time.Second).SearchPatient(context.Background(), url, "1")

	assert.ErrorIs(t, err, his.ErrUnavailable)
}
