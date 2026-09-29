# Hospital Middleware — Agnos back-end assignment

An API between hospital staff and each hospital's HIS (Hospital Information System).
Staff log in, then search patients. **A staff member only ever sees patients of their own hospital.**
If a patient is not in our database yet, the service asks that hospital's HIS by national ID or passport ID,
saves the answer, and returns it.

Stack: Go 1.27 · Gin · GORM · PostgreSQL 17 · Nginx · Docker Compose

## Run it

```bash
docker compose up -d --build
curl localhost:8080/health          # {"status":"ok"}
```

That starts 4 containers:

| Service | What it is |
|---|---|
| `nginx` | public entry, `localhost:8080` → `app` |
| `app` | the Go service |
| `db` | Postgres; schema from `migrations/init.sql`; also on `localhost:5434` for tests |
| `mock-his` | a fake HIS for Hospital A and B (the HIS domain in the assignment doesn't exist), see [Mock HIS](#mock-his) |

Reset everything (deletes the data): `docker compose down -v`

## Run the tests

Tests use the real Postgres from compose. Each test creates its own empty database and drops it afterwards,
so they never touch the app's data. No internet needed — HIS is faked inside the tests.

```bash
docker compose up -d db
go test ./...
```

Use `TEST_DATABASE_URL` to point tests at another Postgres (default
`postgres://agnos:agnos@localhost:5434/postgres?sslmode=disable`).

## API

All request and response bodies are JSON. Every error has the shape `{"error": "message"}`.

| Method | Path | Auth | Success | Errors |
|---|---|---|---|---|
| GET | `/health` | — | `200` | `503` database down |
| POST | `/staff/create` | — | `201` | `400` bad input or unknown hospital · `409` username already used in that hospital |
| POST | `/staff/login` | — | `200` | `400` bad input · `401` wrong username, password or hospital |
| POST | `/patient/search` | `Bearer` token | `200` | `400` bad input · `401` missing/invalid token · `502` HIS unavailable |

### `POST /staff/create`

`hospital` is the hospital code (`hospital-a`, `hospital-b`). Password: 8–72 characters.

```bash
curl -X POST localhost:8080/staff/create -H 'Content-Type: application/json' \
  -d '{"username":"nurse","password":"secret123","hospital":"hospital-a"}'
# 201 {"id":1,"username":"nurse","hospital":"hospital-a"}
```

The same username may exist in different hospitals.

### `POST /staff/login`

```bash
curl -X POST localhost:8080/staff/login -H 'Content-Type: application/json' \
  -d '{"username":"nurse","password":"secret123","hospital":"hospital-a"}'
# 200 {"access_token":"eyJ...","token_type":"Bearer","expires_in":3600}
```

Every failure gives the same `401` message, so the answer doesn't reveal which usernames exist.

### `POST /patient/search`

All fields are optional; filled fields are combined with AND. An empty body returns all patients of your
hospital (max 100).

| Field | Match |
|---|---|
| `national_id`, `passport_id`, `phone_number` | exact |
| `first_name`, `middle_name`, `last_name` | starts with, ignores case, matches the Thai **or** English name |
| `date_of_birth` | exact, `YYYY-MM-DD` |
| `email` | exact, ignores case |

```bash
TOKEN=$(curl -s -X POST localhost:8080/staff/login -H 'Content-Type: application/json' \
  -d '{"username":"nurse","password":"secret123","hospital":"hospital-a"}' | sed 's/.*"access_token":"\([^"]*\)".*/\1/')

curl -X POST localhost:8080/patient/search -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" -d '{"national_id":"1100100100011"}'
```

```json
{
  "patients": [{
    "patient_hn": "A-0001",
    "national_id": "1100100100011", "passport_id": null,
    "first_name_th": "สมชาย", "middle_name_th": null, "last_name_th": "ใจดี",
    "first_name_en": "Somchai", "middle_name_en": null, "last_name_en": "Jaidee",
    "date_of_birth": "1990-05-12", "phone_number": "0811111111",
    "email": "somchai@example.com", "gender": "M"
  }]
}
```

**How the search works:**

1. Search our database, always limited to the hospital in your token.
2. Found → return it.
3. Nothing found and the request has `national_id` or `passport_id` → call your hospital's HIS
   `GET {api_base_url}/patient/search/{id}`:
   - found → save it (insert or update by hospital + HN), search again, return it
   - HIS says 404 → `200 {"patients":[]}`
   - HIS down, slow (> 5 s) or sends bad data → `502` — "HIS is broken" never looks like "no such patient"

## Mock HIS

`mock-his` is plain Nginx serving JSON files, with the same API as the real HIS:
`GET /{hospital}/patient/search/{id}` → `mock-his/data/{hospital}/{id}.json`, or 404.
In compose, `mock-his/seed.sql` points both hospitals' `api_base_url` at it.
Tests don't use it — they start their own fake HIS.

| Hospital | ID | Patient |
|---|---|---|
| hospital-a | `1100100100011` | สมชาย ใจดี, HN `A-0001` |
| hospital-a | `AA1234567` (passport) | John Michael Smith, HN `A-0002` |
| hospital-a | `0000000000000` | HIS returns 503 → our API returns `502` |
| hospital-b | `1100100100011` | the same person as in A, but B's own HN `B-7001` |
| hospital-b | `3100200300044` | มานี มีนา, HN `B-7002` |

Try: log in as a Hospital A staff and search `3100200300044` → `[]`. That patient exists only at Hospital B.

## Project structure

```
cmd/server/main.go     start: config → database → HIS client → router
internal/
  config/              read and check env vars
  database/            open Postgres (GORM)
  model/               Hospital, Staff, Patient, Date
  auth/                JWT issue/parse + middleware for protected routes
  his/                 HTTP client for a hospital's HIS
  staff/               /staff/create, /staff/login
  patient/             /patient/search
  server/              routes
  testutil/            per-test database, request helpers
migrations/init.sql    schema + hospitals seed (used by compose and by tests)
nginx/                 reverse proxy config
mock-his/              fake HIS for the compose demo
```

## Design decisions

- **Hospitals are a table**, not constants. A new hospital is one row with its HIS `api_base_url`.
- **Staff username is unique per hospital** `(hospital_id, username)`, because login takes username +
  password + hospital.
- **Patient HN is unique per hospital** `(hospital_id, patient_hn)`. This is also the upsert key for data from HIS.
- **Names are stored in Thai and English** as HIS sends them. The API has one `first_name`, so search matches
  both languages.
- **The hospital for a search comes only from the JWT**, never from the request body, and it is a condition
  in the SQL, not a filter after loading. A staff member can't reach another hospital's patients or HIS.
- **"HIS down" ≠ "patient not found"** — `502` vs `200 []`, so staff don't wrongly think a patient is new.
- Passwords hashed with bcrypt. Login errors are identical and take the same time.
- JWT accepts only HS256 with an expiry. The service refuses to start with a `JWT_SECRET` under 32 bytes.

## Configuration

| Env | Default in compose | Note |
|---|---|---|
| `DATABASE_URL` | compose Postgres | required |
| `JWT_SECRET` | a dev-only value | **set your own** outside local dev; min 32 bytes |
| `PORT` | `8080` | |
