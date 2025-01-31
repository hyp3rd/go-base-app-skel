GOLANGCI_LINT_VERSION = v1.63.3

GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./.git/*")

# Version environment variable to use in the build process
GITVERSION = $(shell gitversion | jq .SemVer)
GITVERSION_NOT_INSTALLED = "gitversion is not installed: https://github.com/GitTools/GitVersion"
# Set this variable to the value needed for your project, if required, e.g. different from github.com.
GO_PRIVATE=$(shell git config --get remote.origin.url | sed -e 's/git@github.com:/https:\\/\\/github.com\\//' | sed -e 's/http[s]*:\\/\\///' | sed -e 's/\\.git//')

LOCAL_MODULE_PREFIX=your-org/your-project

test:
	go test -v -timeout 5m -cover ./...

update-deps:
	go get -v -u ./...
	go mod tidy

prepare-toolchain:
	$(call check_command_exists,docker) || (echo "Docker is missing, install it before starting to code." && exit 1)

	$(call check_command_exists,git) || (echo "git is not present on the system, install it before starting to code." && exit 1)

	$(call check_command_exists,go) || (echo "golang is not present on the system, download and install it at https://go.dev/dl" && exit 1)

	$(call check_command_exists,gitversion) || (echo "${GITVERSION_NOT_INSTALLED}" && exit 1)

	@echo "Installing gci...\n"
	$(call check_command_exists,gci) || go install github.com/daixiang0/gci@latest

	@echo "Installing gofumpt...\n"
	$(call check_command_exists,gofumpt) || go install mvdan.cc/gofumpt@latest

	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)...\n"
	$(call check_command_exists,golangci-lint) || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" $(GOLANGCI_LINT_VERSION)

	@echo "Installing staticcheck...\n"
	$(call check_command_exists,staticcheck) || go install honnef.co/go/tools/cmd/staticcheck@latest

	@echo "Checking if GOPRIVATE is set correctly and contains ${GO_PRIVATE}n"
	go env GOPRIVATE | grep ${GO_PRIVATE} || (echo "GOPRIVATE does not contain ${GO_PRIVATE}, setting GOPRIVATE" && go env -w GOPRIVATE=${GO_PRIVATE})

	@echo "Checking if pre-commit is installed..."
	pre-commit --version || (echo "pre-commit is not installed, install it with 'pip install pre-commit'" && exit 1)

	@echo "Initializing pre-commit..."
	pre-commit validate-config || pre-commit install && pre-commit install-hooks


lint: prepare-toolchain

	@echo "Running gci..."
	@for file in ${GOFILES_NOVENDOR}; do \
		gci write -s standard -s default -s "prefix(${LOCAL_MODULE_PREFIX})" --skip-vendor --skip-generated $$file; \
	done

	@echo "\nRunning gofumpt..."
	gofumpt -l -w ${GOFILES_NOVENDOR}

	@echo "\nRunning staticcheck..."
	staticcheck ./...

	@echo "\nRunning golangci-lint $(GOLANGCI_LINT_VERSION)..."
	golangci-lint run ./...

run:
	$(call check_command_exists,gitversion) || (echo "${GITVERSION_NOT_INSTALLED}" && exit 1)
	docker compose -f compose.yaml -v down
	export VERSION=${GITVERSION} && docker compose -f $(FILE) up --build -d

# check_command_exists is a helper function that checks if a command exists.
define check_command_exists
@which $(1) > /dev/null 2>&1 || (echo "$(1) command not found" && exit 1)
endef

ifeq ($(call check_command_exists,$(1)),false)
  $(error "$(1) command not found")
endif

# help prints a list of available targets and their descriptions.
help:
	@echo "Available targets:"
	@echo
	@echo "test\t\t\t\tRun all tests in the project."
	@echo "update-deps\t\t\tUpdate all dependencies in the project."
	@echo "lint\t\t\t\tRun the staticcheck and golangci-lint static analysis tools on all packages in the project."
	@echo "run\t\t\t\tRun the project."
	@echo
	@echo "help\t\t\t\tPrint this help message."
	@echo
	@echo "For more information, see the project README."

.PHONY: prepare-toolchain test vet update-deps lint help
