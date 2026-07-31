MODULE      := github.com/garoze/muninn
CMD_DIR     := cmd
BIN_DIR     := bin
CRD_DIR     := config/crd
SAMPLES_DIR := config/samples
PROTO_DIR   := proto
PROTO_SRC   := $(PROTO_DIR)/v1
GEN_DIR     := gen

CONTROLLER_GEN ?= controller-gen
PROTOC         ?= protoc
IMG            ?= muninn:latest
KUBECONFIG     ?= $(HOME)/.kube/config

TENANT ?=
KEYS   ?=

MANAGER_BIN ?= muninn
QUERY_BIN   ?= muninnctl

.PHONY: generate test test-unit test-integration build image load lint \
	fmt vet tidy proto install-crds sample run query clean

# regenerate deepcopy code from kubebuilder markers
generate:
	$(CONTROLLER_GEN) object:headerFile="" paths="./api/..."

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
build: generate
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

# exercises real dependencies (e.g. a k3s cluster); requires KUBECONFIG
test-integration:
	go test ./... -tags=integration -run Integration

image:
	docker build -t $(IMG) .

# import the built image into the local k3s node's containerd store
load:
	docker save $(IMG) | sudo k3s ctr images import -

# generate CRD manifests and apply them to the cluster in $KUBECONFIG
install-crds:
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:dir=$(CRD_DIR)
	kubectl apply -f $(CRD_DIR)

# apply the sample Namespace, Tenant, TenantConfig, and Policy to the cluster
sample:
	kubectl apply -f $(SAMPLES_DIR)/namespace.yaml
	kubectl apply -f $(SAMPLES_DIR)/tenant.yaml
	kubectl apply -f $(SAMPLES_DIR)/tenantconfig.yaml
	kubectl apply -f $(SAMPLES_DIR)/policy.yaml

# patch the sample Tenant's status with placeholder CloudResources data
sample-status:
	kubectl patch tenant arasaka --type=merge --subresource=status \
		--patch-file=$(SAMPLES_DIR)/tenant-status-patch.yaml

# run the server locally against the cluster in $KUBECONFIG
run:
	KUBE_CONFIG_PATH=$(KUBECONFIG) go run ./$(CMD_DIR)/$(MANAGER_BIN)

# query the Muninn Query API, e.g.:
#   make query TENANT=tenant-abc KEYS=TENANT.id,TENANT.runtime
query:
	@if [ -z "$(TENANT)" ] || [ -z "$(KEYS)" ]; then \
		echo "usage: make query TENANT=<tenant-id> KEYS=<comma,separated,keys>"; \
		exit 1; \
	fi
	go run ./$(CMD_DIR)/$(QUERY_BIN) query --tenant $(TENANT) --keys $(KEYS)

clean:
	rm -rf $(BIN_DIR)
