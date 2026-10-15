export GOTOOLCHAIN := go1.27.2
LOCAL_ADMIN := postgres://postgres:local-migrations-only@127.0.0.1:55432/islands?sslmode=disable
.PHONY: setup generate assets build preview test browser check security container
setup:
	docker compose up -d --wait
	npm ci --prefix web --ignore-scripts
	MIGRATION_DATABASE_URL='$(LOCAL_ADMIN)' go run ./cmd/migrate
generate:
	go tool sqlc generate
	go tool tygo generate
	go tool shadcn-templ bundle
	go tool templ generate
assets: generate
	npm run check --prefix web
	npm run build --prefix web
build: assets
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/server ./cmd/server
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/migrate ./cmd/migrate
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/admin ./cmd/admin
preview: assets
	go run ./cmd/preview
test:
	ISLANDS_TEST_DATABASE_URL='$(LOCAL_ADMIN)' go test -race -count=1 ./...
browser:
	npm run test --prefix web
check:
	test -z "$$(gofmt -l cmd internal templates web/assets.go db/migrate.go)"
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --build-tags=acceptance
security:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --enable-only=gosec
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	npm audit --prefix web --audit-level=high
container:
	docker build -t go-islands-starter:local .
