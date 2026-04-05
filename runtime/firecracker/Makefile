.PHONY: all build vet test test-integration lint clean fmt check fetch-kernel verify-kernel image-test-integration image-fetch-test

# Kernel image download/verification targets. The URL and SHA256 here MUST
# stay in sync with DefaultKernelURL / DefaultKernelSHA256 in kernel.go.
KERNEL_URL    ?= https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.10/x86_64/vmlinux-5.10.225
KERNEL_SHA256 ?= 89370d37ec2e0ee8e15df72a131ba04e3a20bba76d4ce07daad15f7e3f8e3c4f
KERNEL_DIR    ?= kernel
KERNEL_FILE   ?= $(KERNEL_DIR)/vmlinux-5.10

all: build vet test

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./... -v -short

test-integration:
	go test -tags=integration ./... -v -timeout 3m

lint:
	@which staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed -- skipping lint"

clean:
	go clean ./...
	rm -f coverage.out

fmt:
	gofmt -w .

check: build vet lint
	@echo "All checks passed"

# fetch-kernel downloads the pinned Firecracker kernel image into $(KERNEL_DIR).
# Uses curl with -fL (fail-on-error, follow-redirects). No-op if the file
# already exists -- use `make clean-kernel` (or remove $(KERNEL_FILE)) to refetch.
fetch-kernel:
	@mkdir -p $(KERNEL_DIR)
	@if [ -f $(KERNEL_FILE) ]; then \
		echo "kernel already present: $(KERNEL_FILE)"; \
	else \
		echo "fetching kernel from $(KERNEL_URL)"; \
		curl -fL -o $(KERNEL_FILE) $(KERNEL_URL); \
	fi

# verify-kernel checks the SHA256 of $(KERNEL_FILE) against $(KERNEL_SHA256).
# Uses sha256sum on Linux and shasum -a 256 on macOS.
verify-kernel:
	@if [ ! -f $(KERNEL_FILE) ]; then \
		echo "kernel not found: $(KERNEL_FILE) (run 'make fetch-kernel' first)"; \
		exit 1; \
	fi
	@echo "verifying $(KERNEL_FILE) against SHA256 $(KERNEL_SHA256)"
	@if command -v sha256sum >/dev/null 2>&1; then \
		ACTUAL=$$(sha256sum $(KERNEL_FILE) | awk '{print $$1}'); \
	else \
		ACTUAL=$$(shasum -a 256 $(KERNEL_FILE) | awk '{print $$1}'); \
	fi; \
	if [ "$$ACTUAL" = "$(KERNEL_SHA256)" ]; then \
		echo "kernel checksum OK"; \
	else \
		echo "kernel checksum MISMATCH: expected $(KERNEL_SHA256), got $$ACTUAL"; \
		exit 1; \
	fi

# image-test-integration runs the Phase 2 integration test that pulls
# a small OCI image end-to-end. Requires network access.
# Uses public.ecr.aws (no anonymous rate limit) for reliability.
image-test-integration:
	go test -tags=integration ./image/... -v -timeout 3m -run TestIntegrationProvision

# image-fetch-test pre-pulls the integration-test OCI image into a local
# directory so the integration test can run offline on subsequent calls.
# Uses crane via `go run` so no extra binary is required.
IMAGE_FETCH_REF ?= public.ecr.aws/docker/library/alpine:3.19
IMAGE_FETCH_DIR ?= image/testdata/oci-cache
image-fetch-test:
	@mkdir -p $(IMAGE_FETCH_DIR)
	@echo "pre-pulling $(IMAGE_FETCH_REF) into $(IMAGE_FETCH_DIR)"
	@go run github.com/google/go-containerregistry/cmd/crane@latest pull \
		--platform linux/amd64 \
		$(IMAGE_FETCH_REF) $(IMAGE_FETCH_DIR)/alpine.tar
	@echo "done: $(IMAGE_FETCH_DIR)/alpine.tar"
