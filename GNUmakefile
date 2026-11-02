default: testacc

# Run unit tests (no API credentials required).
test:
	go test ./... -v -count=1 -timeout=120s

# Run acceptance tests (requires real credentials).
testacc:
	PUSHOVER_API_TOKEN=$(PUSHOVER_API_TOKEN) \
	PUSHOVER_USER_KEY=$(PUSHOVER_USER_KEY) \
	go test ./... -v -count=1 -timeout=120s

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

.PHONY: test testacc build build-local install lint fmt docs
