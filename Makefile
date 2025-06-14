.PHONY: install build run

install:
	go mod tidy

build: install
	go build -o ./build/perf ./cmd/perf

run: build
	teller run --reset --shell -- go run ./cmd/perf

test:
	teller run --reset --shell -- go test -v ./...

sync:
	rclone sync log.md drive:perf/
	rclone sync pkg/openai/prompt drive:perf/
