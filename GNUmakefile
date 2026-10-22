# Makefile for terraform-provider-watchpoint.
# `make` requirements: Go 1.24+, and (for `docs`) the tfplugindocs tool.

BINARY  := terraform-provider-watchpoint
# Bump VERSION to release; `make release` tags v$(VERSION) and CI publishes it.
VERSION := $(strip $(file < VERSION))

.PHONY: build install fmt vet test docs clean tidy release

## build: compile the provider binary into the repo root.
build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

## install: build and place the binary on the Go bin path so a
## dev_overrides block can find it.
install:
	go install -ldflags "-X main.version=$(VERSION)" .

## fmt: gofmt all Go sources in place.
fmt:
	gofmt -w ./internal ./main.go

## vet: run go vet across the module.
vet:
	go vet ./...

## test: run unit tests.
test:
	go test ./... -count=1

## tidy: reconcile go.mod / go.sum.
tidy:
	go mod tidy

## docs: regenerate registry documentation from the schema + examples.
## Requires: go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
docs:
	tfplugindocs generate --provider-name watchpoint

## release: tag the current commit v$(VERSION) and push it; the ci-cd release job does the rest.
release:
	git tag -a v$(VERSION) -m "v$(VERSION)"
	git push origin v$(VERSION)

## clean: remove build artifacts.
clean:
	rm -f $(BINARY) $(BINARY).exe
