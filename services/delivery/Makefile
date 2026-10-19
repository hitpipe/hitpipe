GO ?= go

.PHONY: test vet check

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

check: test vet
