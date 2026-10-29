.PHONY: dev-api dev-web web build tidy

dev-api:
	go run ./cmd/xemail

dev-web:
	cd web && pnpm dev

web:
	cd web && pnpm build

build: web
	go build -o bin/xemail ./cmd/xemail

tidy:
	go mod tidy
