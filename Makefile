APP_BINARY=gallery-server

.PHONY: fmt fmt-check tidy test build run ci smoke-compose compose-up compose-down compose-config

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$(shell gofmt -l cmd internal)" || (printf 'Unformatted files:\n%s\n' "$(shell gofmt -l cmd internal)" && exit 1)

tidy:
	go mod tidy

test:
	go test ./...

build:
	go build -o $(APP_BINARY) ./cmd/server

ci: fmt-check tidy test build compose-config
	@true

smoke-compose:
	bash scripts/compose-smoke-test.sh

run:
	go run ./cmd/server

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

compose-config:
	docker compose --env-file .env.example config
