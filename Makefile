.PHONY: build test-unit test-e2e test run-example clean

## build: compile the browsii binary into the repo root
build:
	@./scripts/build.sh

## test-unit: run unit tests (no browser required)
test-unit:
	@./scripts/test-unit.sh

## test-e2e: build then run end-to-end tests (launches headless browser)
test-e2e:
	@./scripts/test-e2e.sh

## test: run unit tests then e2e tests
test: test-unit test-e2e

## run-example: run the Go client example (examples/go/01_basics.go)
run-example:
	@./scripts/run-example.sh

## clean: remove build artifacts
clean:
	rm -f browsii browsii.exe
