MODULE      := github.com/garoze/muninn
CMD_DIR     := cmd
BIN_DIR     := bin
SAMPLES_DIR := config/samples
MANAGER_DIR := config/manager
RBAC_DIR    := config/rbac
WEBHOOK_DIR := config/webhook
PROTO_DIR   := proto
PROTO_SRC   := $(PROTO_DIR)/v1

.DEFAULT_GOAL := help

PROTOC     ?= protoc
IMG        ?= ghcr.io/garoze/muninn:latest
IMG_REPO   := $(firstword $(subst :, ,$(IMG)))
IMG_TAG    := $(word 2,$(subst :, ,$(IMG)))
KUBECONFIG ?= $(HOME)/.kube/config
KUSTOMIZE  ?= kustomize

NAMESPACE ?=
KEYS      ?=

MANAGER_BIN ?= muninn
QUERY_BIN   ?= muninnctl

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(MODULE)/internal/version.Version=$(VERSION)

.PHONY: help test test-unit test-integration test-e2e test-e2e-csi build image \
	load lint fmt vet tidy proto sample sample-events run query describe \
	deploy undeploy deploy-webhook undeploy-webhook clean

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

# deploys against your real cluster via `make deploy`/`undeploy` and exercises
# it over a port-forward. Requires the image already built and loaded
# (`make image load` — not run automatically here, since `load` needs
# interactive sudo). Not part of `make test` or CI — see docs/design.md.
test-e2e: ## End-to-end against a cluster you already have
	MUNINN_IT_E2E=1 go test ./test/e2e/... -run TestE2E -v -timeout 8m -count=1

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
load: ## Import the image into the local k3s containerd store
	sudo k3s ctr -n k8s.io images import $(BIN_DIR)/image.tar

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

# deploy muninn in-cluster under its own least-privilege ServiceAccount
# (applied in dependency order: namespace, then RBAC, then the Deployment)
deploy: ## Apply the resolver in-cluster
	kubectl apply -f $(MANAGER_DIR)/namespace.yaml
	kubectl apply -f $(RBAC_DIR)/service_account.yaml
	kubectl apply -f $(RBAC_DIR)/role.yaml
	kubectl apply -f $(RBAC_DIR)/role_binding.yaml
	cd $(MANAGER_DIR) && $(KUSTOMIZE) edit set image $(IMG_REPO)=$(IMG)
	kubectl apply -k $(MANAGER_DIR)
	kubectl apply -f $(MANAGER_DIR)/service.yaml

# tear down everything `make deploy` created (reverse order; the namespace
# delete alone would cascade the rest, but tearing down explicitly keeps the
# ClusterRole/ClusterRoleBinding — cluster-scoped, so not caught by that
# cascade — from being left behind)
undeploy: ## Tear down the resolver
	kubectl delete -f $(MANAGER_DIR)/service.yaml --ignore-not-found
	kubectl delete -k $(MANAGER_DIR) --ignore-not-found
	kubectl delete -f $(RBAC_DIR)/role_binding.yaml --ignore-not-found
	kubectl delete -f $(RBAC_DIR)/role.yaml --ignore-not-found
	kubectl delete -f $(RBAC_DIR)/service_account.yaml --ignore-not-found
	kubectl delete -f $(MANAGER_DIR)/namespace.yaml --ignore-not-found

# deploy the mutating admission webhook. Requires cert-manager already
# installed on the cluster (external prerequisite, not managed by this repo)
# and `make deploy` already applied (needs the muninn-system namespace and
# the gRPC Service the injected init container/sidecar dial). Applied in
# dependency order: Issuer/Certificate first so cert-manager has time to
# issue the Secret the Deployment mounts, then ServiceAccount/RBAC, then
# Service/Deployment, then the MutatingWebhookConfiguration last so the API
# server doesn't start routing admission requests before the backend exists.
#
# role_spc_writer(_binding).yaml grants create and patch on SecretProviderClass,
# needed only for SECRET_SPC_MODE=Create (this repo's own reference
# deployment default - see deployment.yaml). A Reference-mode deployment in
# an environment that doesn't want the webhook able to create resources in
# consumer namespaces should drop those two `kubectl apply` lines - role.yaml
# alone (get/list/watch configmaps, get secretproviderclasses) is sufficient
# for that mode.
deploy-webhook: ## Apply the mutating admission webhook
	kubectl apply -f $(WEBHOOK_DIR)/issuer.yaml
	kubectl apply -f $(WEBHOOK_DIR)/certificate.yaml
	kubectl apply -f $(WEBHOOK_DIR)/service_account.yaml
	kubectl apply -f $(WEBHOOK_DIR)/role.yaml
	kubectl apply -f $(WEBHOOK_DIR)/role_binding.yaml
	kubectl apply -f $(WEBHOOK_DIR)/role_spc_writer.yaml
	kubectl apply -f $(WEBHOOK_DIR)/role_spc_writer_binding.yaml
	kubectl apply -f $(WEBHOOK_DIR)/service.yaml
	cd $(WEBHOOK_DIR) && $(KUSTOMIZE) edit set image $(IMG_REPO)=$(IMG)
	kubectl apply -k $(WEBHOOK_DIR)
	kubectl apply -f $(WEBHOOK_DIR)/webhook.yaml

# tear down everything `make deploy-webhook` created (reverse order; the
# MutatingWebhookConfiguration goes first so the API server stops routing
# admission requests here before the backend disappears)
undeploy-webhook: ## Tear down the mutating admission webhook
	kubectl delete -f $(WEBHOOK_DIR)/webhook.yaml --ignore-not-found
	kubectl delete -k $(WEBHOOK_DIR) --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/service.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/role_spc_writer_binding.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/role_spc_writer.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/role_binding.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/role.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/service_account.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/certificate.yaml --ignore-not-found
	kubectl delete -f $(WEBHOOK_DIR)/issuer.yaml --ignore-not-found

clean: ## Remove bin/
	rm -rf $(BIN_DIR)

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
