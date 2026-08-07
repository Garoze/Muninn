# Troubleshooting

Known issues encountered while building and running Muninn, recorded so
the diagnosis doesn't have to be rediscovered. This is not an on-call
runbook: Muninn is a portfolio/reference implementation, not an
operated production service. It's a record of real problems hit during
development and how to recognize them again.

## Pod stuck with `ErrImageNeverPull`

**Symptom:** After `make deploy`, `kubectl get pods -n muninn-system`
shows the pod stuck in `ErrImageNeverPull` instead of reaching
`1/1 Running`.

**Cause:** The Deployment manifest uses `imagePullPolicy: Never`, which
requires an exact string match against whatever image reference is
already in the node's containerd store. `make load` tags the image with
a `localhost/` prefix before importing it (Podman applies this prefix to
local images automatically; Docker doesn't, so the tag is added
explicitly to keep the reference identical either way). If the image
was built or loaded through a different path than `make image`/`make
load`, the reference in containerd's store may not match what the
manifest expects.

**Fix:** Run `make image load` (in that order) and confirm the image
exists in containerd's store before re-running `make deploy`. This
failure mode is also detected and reported explicitly by the end-to-end
test (`make test-e2e`) rather than surfacing as an unrelated timeout.

## Killing a locally-run process doesn't stop it

**Symptom:** Running Muninn via `go run ./cmd/muninn` and killing the
recorded PID doesn't actually stop the server: it keeps holding its
port. The same thing happens with a `kubectl port-forward` started in
the background: it outlives the shell that started it.

**Cause:** `go run` compiles a binary and execs it as a child process;
killing the `go run` process itself doesn't necessarily kill that child.
The same applies to any backgrounded subprocess whose lifecycle isn't
explicitly supervised.

**Fix:** For manual verification, prefer `go build` followed by running
the resulting binary directly, so the PID in hand is the PID actually
listening. For `kubectl port-forward`, confirm it's actually terminated
rather than assuming a single `kill` on the shell job was sufficient.

## Process crashes on first use of a metric, not at startup

**Symptom:** The server starts and runs normally, then panics the first
time one specific operation executes: not during startup, not under
load in general, only the first time that one code path runs.

**Cause:** A Prometheus metric's declared label set and the label values
supplied at its call site have to match in count exactly. A mismatch
compiles fine and doesn't fail at startup: it only panics the first
time that specific metric is actually recorded, which can make the
crash look unrelated to whatever change introduced it.

**Fix:** When adding or changing a metric, verify every call site
supplying label values against that metric's declared label list. This
is exactly the class of bug the pull request template's metrics
checklist item exists to catch before merge, not after.

## `make test-integration` fails before any test runs

**Symptom:** `make test-integration` fails immediately with an error
about missing binaries or an unset environment variable, before any
test logic executes.

**Cause:** The integration tests run against a real, throwaway
`etcd`/`kube-apiserver` pair provisioned by `setup-envtest`, located via
the `KUBEBUILDER_ASSETS` environment variable. If that variable isn't
set, or points at a stale path, the test binaries have no control plane
to start against.

**Fix:** `export KUBEBUILDER_ASSETS=$(setup-envtest use -p path)` (see
the README's Prerequisites section) before running `make
test-integration`.
