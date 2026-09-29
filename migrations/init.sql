CREATE TABLE hospitals (
    id           BIGSERIAL PRIMARY KEY,
    code         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    api_base_url TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- username is unique per hospital, not globally: login takes (username, password, hospital)
CREATE TABLE staffs (
    id            BIGSERIAL PRIMARY KEY,
    hospital_id   BIGINT NOT NULL REFERENCES hospitals(id),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (hospital_id, username)
);

-- mirrors the HIS response; names kept in both TH and EN as HIS sends them
CREATE TABLE patients (
    id             BIGSERIAL PRIMARY KEY,
    hospital_id    BIGINT NOT NULL REFERENCES hospitals(id),
    patient_hn     TEXT NOT NULL,
    national_id    TEXT,
    passport_id    TEXT,
    first_name_th  TEXT,
    middle_name_th TEXT,
    last_name_th   TEXT,
    first_name_en  TEXT,
    middle_name_en TEXT,
    last_name_en   TEXT,
    date_of_birth  DATE,
    phone_number   TEXT,
    email          TEXT,
    gender         CHAR(1) CHECK (gender IN ('M', 'F')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (hospital_id, patient_hn)
);

-- every search is scoped by hospital_id, so it leads each index
CREATE INDEX idx_patients_national_id  ON patients (hospital_id, national_id);
CREATE INDEX idx_patients_passport_id  ON patients (hospital_id, passport_id);
CREATE INDEX idx_patients_last_name_th ON patients (hospital_id, last_name_th);
CREATE INDEX idx_patients_last_name_en ON patients (hospital_id, last_name_en);

INSERT INTO hospitals (code, name, api_base_url) VALUES
    ('hospital-a', 'Hospital A', 'https://hospital-a.api.co.th'),
    ('hospital-b', 'Hospital B', 'https://hospital-b.api.co.th');
