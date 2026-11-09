default: test

# Run unit tests (no API credentials required). Includes the lifecycle contract
# against whichever terraform binary is on PATH.
test:
	go test ./... -v -count=1 -timeout=300s

# Lifecycle contract only. httptest doubles; no Pushover calls and no TF_ACC.
test-lifecycle:
	go test ./internal/provider -count=1 -timeout=300s -run '^TestLifecycleContract'

test-lifecycle-terraform:
	TF_ACC_TERRAFORM_PATH="$(shell command -v terraform)" $(MAKE) test-lifecycle

test-lifecycle-tofu:
	TF_ACC_TERRAFORM_PATH="$(shell command -v tofu)" $(MAKE) test-lifecycle

# Live acceptance tests. TF_ACC=1 is required or resource.Test skips them.
# This calls the real Pushover API when PUSHOVER_API_TOKEN is set. License
# assignment tests stay skipped unless PUSHOVER_ALLOW_PAID_TESTS=1.
testacc:
	TF_ACC=1 \
	PUSHOVER_API_TOKEN=$(PUSHOVER_API_TOKEN) \
	PUSHOVER_USER_KEY=$(PUSHOVER_USER_KEY) \
	go test ./... -v -count=1 -timeout=300s

# Build the provider binary.
build:
	go build ./...

# Build a local development binary.
build-local:
	go build -o terraform-provider-pushover .

# Install the provider for local development.
# Installs to ~/.terraform.d/plugins/registry.terraform.io/Josh-Archer/pushover/0.0.1/<OS>_<ARCH>/
install: build-local
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/Josh-Archer/pushover/0.0.1/$$(go env GOOS)_$$(go env GOARCH)
	mv terraform-provider-pushover ~/.terraform.d/plugins/registry.terraform.io/Josh-Archer/pushover/0.0.1/$$(go env GOOS)_$$(go env GOARCH)/terraform-provider-pushover_v0.0.1

# Lint the code.
lint:
	go vet ./...

# Format the code.
fmt:
	gofmt -s -w .

# Validate documentation using tfplugindocs.
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 validate --provider-name pushover

.PHONY: test test-lifecycle test-lifecycle-terraform test-lifecycle-tofu testacc build build-local install lint fmt docs
