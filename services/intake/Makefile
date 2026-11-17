GO ?= go
MODULE := ./cmd/intake-contractcheck
BUILD_DIR ?= .build
COMMIT ?= unknown
DIRTY ?= unknown

.PHONY: fmt test vet check dev release

fmt:
	$(GO)fmt -w $$(find . -name '*.go' -type f)

test:
	$(GO) test -count=1 ./...

vet:
	$(GO) vet ./...

check: vet test

dev:
	mkdir -p $(BUILD_DIR)/dev
	$(GO) build -gcflags='all=-N -l' -ldflags='-X main.version=pr12 -X main.commit=$(COMMIT) -X main.dirty=$(DIRTY) -X main.buildMode=dev' -o $(BUILD_DIR)/dev/intake-contractcheck $(MODULE)

release:
	mkdir -p $(BUILD_DIR)/release
	$(GO) build -trimpath -ldflags='-X main.version=pr12 -X main.commit=$(COMMIT) -X main.dirty=$(DIRTY) -X main.buildMode=release' -o $(BUILD_DIR)/release/intake-contractcheck $(MODULE)
