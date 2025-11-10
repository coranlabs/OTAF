# Copyright 2025-2026 coRAN LABS Private Limited
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

SHELL := /bin/bash
SDK_ROOT := $(shell pwd)
SCRATCH := $(shell mktemp -d -u -t otaf-rapp-sdk-XXXXXX)

.PHONY: help
help: ## Show the available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS=":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile every package and the CLI
	go build ./...
	go build -o bin/rappctl ./cmd/rappctl

.PHONY: test
test: ## Run the tests
	go test ./...

.PHONY: race
race: ## Run the tests under the race detector
	go test -race ./...

.PHONY: cover
cover: ## Report test coverage per package
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -20

.PHONY: fmt
fmt: ## Format the source
	gofmt -w .

.PHONY: lint
lint: ## Check formatting, vet and lint
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "needs gofmt:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	@command -v golangci-lint >/dev/null && golangci-lint run || \
		echo "golangci-lint not installed, skipping"

.PHONY: e2e
e2e: build ## Generate an rApp and take it through to a validated package
	@rm -rf $(SCRATCH) && mkdir -p $(SCRATCH)
	cd $(SCRATCH) && $(SDK_ROOT)/bin/rappctl new e2e-rapp \
		--module github.com/coranlabs/e2e-rapp --sdk-path $(SDK_ROOT)
	cd $(SCRATCH)/e2e-rapp && go mod tidy && go build ./... && go vet ./...
	cd $(SCRATCH)/e2e-rapp && $(SDK_ROOT)/bin/rappctl package
	cd $(SCRATCH)/e2e-rapp && $(SDK_ROOT)/bin/rappctl validate dist/e2e-rapp-0.1.0.csar
	@command -v helm >/dev/null && helm lint $(SCRATCH)/e2e-rapp/deploy/helm/e2e-rapp || true
	@rm -rf $(SCRATCH)
	@echo "end to end: ok"

.PHONY: check
check: lint race e2e ## Everything CI runs

.PHONY: clean
clean: ## Remove build output
	rm -rf bin coverage.out
