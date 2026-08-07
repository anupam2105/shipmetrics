# Local dev DSN — matches docker-compose.yml.
db_dsn := "postgres://shipmetrics:shipmetrics@localhost:5432/shipmetrics?sslmode=disable"

default:
    @just --list

test:
    go test -race -count=1 ./...

test-integration:
    TEST_DATABASE_URL="{{db_dsn}}" go test -race -count=1 ./...

lint:
    golangci-lint run

check: lint test

run:
    SHIPMETRICS_DATABASE_URL="{{db_dsn}}" go run ./cmd/shipmetrics

# One-shot local dev: start Postgres if needed, then run the app.
dev: db-up
    @echo "waiting for Postgres to become ready..."
    @until docker compose exec -T postgres pg_isready -U shipmetrics -d shipmetrics > /dev/null 2>&1; do sleep 1; done
    SHIPMETRICS_DATABASE_URL="{{db_dsn}}" go run ./cmd/shipmetrics

build:
    CGO_ENABLED=0 go build -o bin/shipmetrics ./cmd/shipmetrics

fmt:
    gofmt -s -w .
    goimports -w .

tidy:
    go mod tidy

clean:
    rm -rf bin/

# --- Postgres (local dev) ---
db-up:
    docker compose up -d postgres
    @echo "Postgres starting up — DSN: {{db_dsn}}"

db-down:
    docker compose down

db-logs:
    docker compose logs -f postgres

db-psql:
    docker compose exec postgres psql -U shipmetrics -d shipmetrics
