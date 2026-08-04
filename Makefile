MODULE      := github.com/garoze/muninn
CMD_DIR     := cmd
BIN_DIR     := bin
SAMPLES_DIR := config/samples
MANAGER_DIR := config/manager
RBAC_DIR    := config/rbac
PROTO_DIR   := proto
PROTO_SRC   := $(PROTO_DIR)/v1
GEN_DIR     := gen

PROTOC          ?= protoc
IMG             ?= muninn:latest
KUBECONFIG      ?= $(HOME)/.kube/config
CONTAINER_ENGINE ?= $(shell command -v podman >/dev/null 2>&1 && echo podman || echo docker)

NAMESPACE ?=
KEYS      ?=

MANAGER_BIN ?= muninn
QUERY_BIN   ?= muninnctl

.PHONY: test test-unit test-integration test-e2e build image load lint \
	fmt vet tidy proto sample run query describe deploy undeploy clean

# regenerate Go code (message types + gRPC stubs) from proto/v1/*.proto
# requires: protoc, protoc-gen-go, protoc-gen-go-grpc on $PATH
proto:
	$(PROTOC) \
		--proto_path=$(PROTO_SRC) \
		--go_out=. --go_opt=module=$(MODULE) \
		--go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
		$(PROTO_SRC)/discovery.proto

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# compile every cmd/ entrypoint into bin/; falls back to a plain
# typecheck build while no entrypoints exist yet
build:
	@found=0; \
	if [ -d $(CMD_DIR) ]; then \
		for d in $(CMD_DIR)/*/; do \
			[ -n "$$(ls $$d*.go 2>/dev/null)" ] || continue; \
			found=1; \
			name=$$(basename $$d); \
			echo "==> building $$name"; \
			go build -o $(BIN_DIR)/$$name ./$$d; \
		done; \
	fi; \
	if [ "$$found" = "0" ]; then \
		echo "==> no cmd/ entrypoints with Go files yet, compiling packages only"; \
		go build ./...; \
	fi

test: test-unit test-integration

# unit tests only, no cluster or other external dependencies required
test-unit:
	go test ./... -short

# exercises envtest (a local, throwaway control plane); requires KUBEBUILDER_ASSETS
# (see https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest and `setup-envtest`)
test-integration:
	MUNINN_IT_ENVTEST=1 go test ./test/... -run TestWatcherProjection -v -count=1

# deploys against your real cluster via `make deploy`/`undeploy` and exercises
# it over a port-forward. Requires the image already built and loaded
# (`make image load` — not run automatically here, since `load` needs
# interactive sudo). Not part of `make test` or CI — see docs/design.md.
test-e2e:
	MUNINN_IT_E2E=1 go test ./test/e2e/... -run TestE2E -v -timeout 5m -count=1

# override the detected engine with: make image CONTAINER_ENGINE=docker
image:
	$(CONTAINER_ENGINE) build -t $(IMG) .

# import the built image into the local k3s node's containerd store. Tag
# with an explicit localhost/ prefix first so config/manager/deployment.yaml's
# image reference matches regardless of which engine built it — Podman
# applies this prefix to local images automatically, Docker does not.
load:
	$(CONTAINER_ENGINE) tag $(IMG) localhost/$(IMG)
	$(CONTAINER_ENGINE) save localhost/$(IMG) | sudo k3s ctr images import -

# apply the sample Namespace and ConfigMap to the cluster
sample:
	kubectl apply -f $(SAMPLES_DIR)/namespace.yaml
	kubectl apply -f $(SAMPLES_DIR)/configmap.yaml

# run the server locally against the cluster in $KUBECONFIG
run:
	KUBE_CONFIG_PATH=$(KUBECONFIG) go run ./$(CMD_DIR)/$(MANAGER_BIN) serve

# query the Muninn Query API, e.g.:
#   make query NAMESPACE=arasaka KEYS=LOG_LEVEL,FEATURE_DARKMODE
query:
	@if [ -z "$(NAMESPACE)" ] || [ -z "$(KEYS)" ]; then \
		echo "usage: make query NAMESPACE=<namespace> KEYS=<comma,separated,keys>"; \
		exit 1; \
	fi
	go run ./$(CMD_DIR)/$(QUERY_BIN) query --namespace $(NAMESPACE) --keys $(KEYS)

# list the active configuration sources
describe:
	go run ./$(CMD_DIR)/$(QUERY_BIN) describe

# deploy muninn in-cluster under its own least-privilege ServiceAccount
# (applied in dependency order: namespace, then RBAC, then the Deployment)
deploy:
	kubectl apply -f $(MANAGER_DIR)/namespace.yaml
	kubectl apply -f $(RBAC_DIR)/service_account.yaml
	kubectl apply -f $(RBAC_DIR)/role.yaml
	kubectl apply -f $(RBAC_DIR)/role_binding.yaml
	kubectl apply -f $(MANAGER_DIR)/deployment.yaml
	kubectl apply -f $(MANAGER_DIR)/service.yaml

# tear down everything `make deploy` created (reverse order; the namespace
# delete alone would cascade the rest, but tearing down explicitly keeps the
# ClusterRole/ClusterRoleBinding — cluster-scoped, so not caught by that
# cascade — from being left behind)
undeploy:
	kubectl delete -f $(MANAGER_DIR)/service.yaml --ignore-not-found
	kubectl delete -f $(MANAGER_DIR)/deployment.yaml --ignore-not-found
	kubectl delete -f $(RBAC_DIR)/role_binding.yaml --ignore-not-found
	kubectl delete -f $(RBAC_DIR)/role.yaml --ignore-not-found
	kubectl delete -f $(RBAC_DIR)/service_account.yaml --ignore-not-found
	kubectl delete -f $(MANAGER_DIR)/namespace.yaml --ignore-not-found

clean:
	rm -rf $(BIN_DIR)
