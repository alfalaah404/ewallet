# ewallet backend — build, run, test
SHELL := /bin/bash
GO := /usr/local/go/bin/go
COVER_PKG := ./internal/...
COVER_FILE := cover.out
COVER_MIN := 90

# Load .env.test (TEST_DATABASE_URL, TEST_REDIS_ADDR) if present.
ifneq (,$(wildcard .env.test))
include .env.test
export
endif

.PHONY: build run test cover cover-html clean

build:
	$(GO) build -o bin/api ./cmd/api

run:
	$(GO) run ./cmd/api

## test: run all tests with the race detector
test:
	$(GO) test -race ./...

## cover: run tests with coverage and enforce >= $(COVER_MIN)% on ./internal/...
cover:
	$(GO) test -race -covermode=atomic -coverpkg=$(COVER_PKG) -coverprofile=$(COVER_FILE) ./internal/...
	@echo "----------------------------------------"
	@$(GO) tool cover -func=$(COVER_FILE) | tail -n 1
	@total=$$($(GO) tool cover -func=$(COVER_FILE) | grep total | grep -Eo '[0-9]+\.[0-9]+'); \
	echo "Total coverage: $$total% (minimum $(COVER_MIN)%)"; \
	pass=$$(awk -v t="$$total" -v m=$(COVER_MIN) 'BEGIN{print (t>=m)?1:0}'); \
	if [ "$$pass" -ne 1 ]; then echo "FAIL: coverage below $(COVER_MIN)%"; exit 1; fi; \
	echo "PASS: coverage gate met"

cover-html: cover
	$(GO) tool cover -html=$(COVER_FILE) -o coverage.html
	@echo "wrote coverage.html"

clean:
	rm -f $(COVER_FILE) coverage.html bin/api
