MODULE      := github.com/garoze/muninn
CMD_DIR     := cmd
BIN_DIR     := bin
SAMPLES_DIR := config/samples
CHART_DIR   := charts/muninn
HELM_DOCS_VERSION := v1.14.2
LOCAL_REGISTRY_NAME := muninn-registry
LOCAL_REGISTRY_PORT ?= 5000
LOCAL_IMAGE_REPO    := localhost:$(LOCAL_REGISTRY_PORT)/muninn
LOCAL_IMAGE_TAG     ?= e2e
PROTO_DIR   := proto
PROTO_SRC   := $(PROTO_DIR)/v1

.DEFAULT_GOAL := help

PROTOC     ?= protoc
IMG        ?= ghcr.io/garoze/muninn:latest
IMG_REPO   := $(firstword $(subst :, ,$(IMG)))
IMG_TAG    := $(word 2,$(subst :, ,$(IMG)))
KUBECONFIG ?= $(HOME)/.kube/config

# Release name and the namespace it installs into, distinct from NAMESPACE
# below - that one is the consumer namespace the query targets resolve for.
RELEASE           ?= muninn
RELEASE_NAMESPACE ?= muninn-system
# Extra values for deploy, e.g. HELM_ARGS="--set webhook.enabled=false".
HELM_ARGS ?=

NAMESPACE ?=
KEYS      ?=

MANAGER_BIN ?= muninn
QUERY_BIN   ?= muninnctl

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(MODULE)/internal/version.Version=$(VERSION)

.PHONY: help test test-unit test-integration test-e2e test-e2e-csi build image \
	load lint fmt vet tidy proto sample sample-events run query describe \
	chart-deps chart-docs deploy undeploy clean registry registry-stop push

# regenerate Go code (message types + gRPC stubs) from proto/v1/*.proto
# requires: protoc, protoc-gen-go, protoc-gen-go-grpc on $PATH
proto: ## Regenerate gRPC stubs from proto/v1 (requires protoc)
	$(PROTOC) \
		--proto_path=$(PROTO_SRC) \
		--go_out=. --go_opt=module=$(MODULE) \
		--go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
		$(PROTO_SRC)/discovery.proto

fmt: ## gofmt -l -w .
	gofmt -l -w .

vet: ## go vet ./...
	go vet ./...

lint: ## golangci-lint run ./... (the check CI runs)
	golangci-lint run ./...

tidy: ## go mod tidy
	go mod tidy

# compile every cmd/ entrypoint into bin/; falls back to a plain
# typecheck build while no entrypoints exist yet
build: ## Compile every cmd/ entrypoint into bin/
	@found=0; \
	if [ -d $(CMD_DIR) ]; then \
		for d in $(CMD_DIR)/*/; do \
			[ -n "$$(ls $$d*.go 2>/dev/null)" ] || continue; \
			found=1; \
			name=$$(basename $$d); \
			echo "==> building $$name"; \
			go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$name ./$$d; \
		done; \
	fi; \
	if [ "$$found" = "0" ]; then \
		echo "==> no cmd/ entrypoints with Go files yet, compiling packages only"; \
		go build ./...; \
	fi

test: test-unit test-integration ## Run the unit and integration tiers

# unit tests only, no cluster or other external dependencies required
test-unit: ## Unit tests only, no cluster required
	go test ./... -short

# exercises envtest (a local, throwaway control plane); requires KUBEBUILDER_ASSETS
# (see https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest and `setup-envtest`)
# ./cmd/muninn/... is included alongside ./test/integration/envtest/... since
# some envtest-gated tests live there instead - they exercise unexported
# package-main internals no external test package can import.
test-integration: ## Integration tests against a throwaway control plane
	MUNINN_IT_ENVTEST=1 go test ./test/integration/envtest/... ./cmd/muninn/... -v -count=1

# installs the chart against your real cluster and exercises it over a
# port-forward. `push` builds the image into a registry the node pulls from,
# so this needs no root; override MUNINN_E2E_IMAGE for a cluster that cannot
# reach that registry. Not part of `make test` or CI - see docs/design.md.
test-e2e: push ## End-to-end against a cluster you already have
	MUNINN_IT_E2E=1 MUNINN_E2E_IMAGE=$(LOCAL_IMAGE_REPO):$(LOCAL_IMAGE_TAG) \
	go test ./test/e2e/... -run TestE2E -v -timeout 8m -count=1

# provisions its own disposable kind cluster and tears it down after -
# unlike test-e2e, needs no existing cluster or pre-loaded image, but does
# need kind/podman/helm/kubectl on PATH. Heavier (several minutes); not
# part of `make test` or CI.
test-e2e-csi: ## End-to-end CSI secret delivery on a disposable kind cluster
	MUNINN_IT_CSI_E2E=1 go test ./test/e2e/... -run TestCSIE2E -v -timeout 15m -count=1

# builds for the host's own platform and writes a tarball rather than
# pushing - --tags/--bare together are verified working despite ko's own
# help text warning they may not be; see .ko.yaml for the pinned base image
# and the three OCI labels' rationale
image: ## Build the container image (via ko) into bin/image.tar
	@mkdir -p $(BIN_DIR)
	KO_DOCKER_REPO=$(IMG_REPO) VERSION=$(VERSION) \
	ko build ./$(CMD_DIR)/$(MANAGER_BIN) --tarball=$(BIN_DIR)/image.tar --push=false --bare --sbom=none \
		--platform=$(shell go env GOOS)/$(shell go env GOARCH) --tags=$(IMG_TAG) \
		--image-label org.opencontainers.image.source=https://github.com/Garoze/Muninn \
		--image-label org.opencontainers.image.description="Kubernetes-native runtime configuration resolver" \
		--image-label org.opencontainers.image.licenses=MIT

# import the built image into the local k3s node's containerd store, in the
# k8s.io namespace specifically - the kubelet/CRI never looks at ctr's
# default namespace, so importing without -n k8s.io leaves the image
# invisible to Pods despite `ctr images list` showing it
load: ## Import the image into the local k3s containerd store (needs root)
	sudo k3s ctr -n k8s.io images import $(BIN_DIR)/image.tar

# Build straight into a registry the cluster can pull from, which needs no
# root: importing into k3s's containerd does, because its socket is owned by
# root, and that is the only reason `load` above asks for a password. A node
# sharing this host's network namespace - k3s, minikube --driver=none, kind
# with the port mapped - resolves localhost:$(LOCAL_REGISTRY_PORT) to the
# registry started here. A cluster elsewhere cannot, and wants `image` and
# `load` instead.
registry: ## Start a local OCI registry for the cluster to pull from
	@if [ -z "$$($(CONTAINER_ENGINE) ps -q -f name=$(LOCAL_REGISTRY_NAME))" ]; then \
		$(CONTAINER_ENGINE) run -d --rm -p $(LOCAL_REGISTRY_PORT):5000 \
			--name $(LOCAL_REGISTRY_NAME) docker.io/library/registry:2 >/dev/null; \
		echo "started $(LOCAL_REGISTRY_NAME) on localhost:$(LOCAL_REGISTRY_PORT)"; \
	else \
		echo "$(LOCAL_REGISTRY_NAME) already running on localhost:$(LOCAL_REGISTRY_PORT)"; \
	fi

registry-stop: ## Stop the local OCI registry
	-@$(CONTAINER_ENGINE) rm -f $(LOCAL_REGISTRY_NAME) >/dev/null 2>&1

push: registry ## Build and push the image to the local registry (no sudo)
	KO_DOCKER_REPO=$(LOCAL_IMAGE_REPO) VERSION=$(VERSION) \
	ko build ./$(CMD_DIR)/$(MANAGER_BIN) --bare --sbom=none --insecure-registry \
		--platform=$(shell go env GOOS)/$(shell go env GOARCH) --tags=$(LOCAL_IMAGE_TAG) \
		--image-label org.opencontainers.image.source=https://github.com/Garoze/Muninn \
		--image-label org.opencontainers.image.description="Kubernetes-native runtime configuration resolver" \
		--image-label org.opencontainers.image.licenses=MIT

# apply the sample Namespace and ConfigMap to the cluster
sample: ## Apply the sample Namespace and labeled ConfigMap
	kubectl apply -f $(SAMPLES_DIR)/namespace.yaml
	kubectl apply -f $(SAMPLES_DIR)/configmap.yaml

# apply the sample Role/RoleBinding granting the arasaka namespace's default
# ServiceAccount events.k8s.io create - separate from `sample` above since
# it's specific to CSI secret-drift Event visibility, not the core resolver
# `sample` demonstrates. Requires `make sample` (or an equivalent arasaka
# namespace) already applied.
sample-events: ## Apply the sample RBAC for secret-drift Events
	kubectl apply -f $(SAMPLES_DIR)/event_writer_role.yaml
	kubectl apply -f $(SAMPLES_DIR)/event_writer_role_binding.yaml

# run the server locally against the cluster in $KUBECONFIG
run: ## Run the resolver locally against $KUBECONFIG
	KUBE_CONFIG_PATH=$(KUBECONFIG) go run ./$(CMD_DIR)/$(MANAGER_BIN) serve

# query the Muninn Query API, e.g.:
#   make query NAMESPACE=arasaka KEYS=LOG_LEVEL,FEATURE_DARKMODE
query: ## Query keys: make query NAMESPACE=<ns> KEYS=<a,b,c>
	@if [ -z "$(NAMESPACE)" ] || [ -z "$(KEYS)" ]; then \
		echo "usage: make query NAMESPACE=<namespace> KEYS=<comma,separated,keys>"; \
		exit 1; \
	fi
	go run ./$(CMD_DIR)/$(QUERY_BIN) query --namespace $(NAMESPACE) --keys $(KEYS)

# list the active configuration sources
describe: ## List the active configuration sources
	go run ./$(CMD_DIR)/$(QUERY_BIN) describe

# vendor the subchart archives into $(CHART_DIR)/charts, which is gitignored.
# Helm refuses to render or install a chart whose declared dependencies are
# missing from disk even when every one of them is condition-disabled, as
# they are by default here. `update` rather than `build` because it resolves
# the repository URLs straight from Chart.yaml, where `build` goes through
# helm's registered repository list and fails on a machine that has never
# added them. Exact version pins keep the resolution deterministic, so
# Chart.lock does not churn.
chart-deps: ## Vendor the chart's subchart archives
	helm dependency update $(CHART_DIR)

# The chart's README is generated from values.yaml's comments, so the values
# and their documentation cannot drift apart: there is only one copy of the
# text. CI regenerates and fails on a diff, which is what makes that true
# rather than merely intended. Pinned rather than @latest, same reasoning as
# every other tool here; --skip-version-footer keeps a helm-docs bump from
# rewriting the file with no content change.
chart-docs: ## Regenerate the chart's README from values.yaml
	go install github.com/norwoodj/helm-docs/cmd/helm-docs@$(HELM_DOCS_VERSION)
	helm-docs --chart-search-root $(CHART_DIR) --sort-values-order file --skip-version-footer

# install or upgrade the chart against the cluster in $KUBECONFIG. Both roles
# come from one release, so the webhook is a value rather than a second
# deploy step:
#   make deploy HELM_ARGS="--set webhook.enabled=false"
# which is also the first phase of the two-phase install a cluster whose
# cert-manager is not already serving needs - see charts/muninn/values.yaml
# for that sequence and every other value this accepts.
deploy: chart-deps ## Install or upgrade the chart in-cluster
	helm upgrade --install $(RELEASE) $(CHART_DIR) \
		--namespace $(RELEASE_NAMESPACE) --create-namespace \
		--set image.repository=$(IMG_REPO) --set image.tag=$(IMG_TAG) \
		$(HELM_ARGS)

# helm removes the ClusterRole/ClusterRoleBinding along with everything else,
# since they are release resources rather than something a namespace delete
# would have to cascade to.
undeploy: ## Uninstall the chart
	helm uninstall $(RELEASE) --namespace $(RELEASE_NAMESPACE) --ignore-not-found

clean: ## Remove bin/
	rm -rf $(BIN_DIR)

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
