<!-- Reference: README.md "Architecture", docs/design.md, docs/adr/, CONTRIBUTING.md -->

## Description

<!-- What does this PR do? Why? Include any relevant context, trade-offs, or design decisions. -->

## Type of Change

<!-- Check all that apply. -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation update
- [ ] API / proto change
- [ ] Dependency update
- [ ] Refactor / technical debt reduction
- [ ] Other (describe below)

## Related Issues

<!-- Link any related issues. Use "Closes #123" to auto-close on merge. -->

## Review Checklist

> Authors should pre-fill this checklist before requesting review;
> reviewers should verify each item.

- [ ] **No real/sensitive data** — No real credentials, endpoints, or
      identifying data introduced anywhere (samples/fixtures/docs use
      placeholder values only, e.g. `config/samples/`).
- [ ] **Domain/transport boundary** — `internal/app` still imports no
      `grpc`/`k8s.io/*`/generated proto types. Translation happens only at
      the edges (`internal/kube` in, `internal/transport/grpc` out).
- [ ] **Metrics label cardinality** — Every `WithLabelValues(...)` call
      site matches the label count declared for that metric in
      `internal/observability/metrics.go`. (This repo has shipped this
      exact bug before — it crashes the process on first use, not at
      startup.)
- [ ] **Proto regen verified** — If `proto/v1/discovery.proto` changed:
      `make proto` was run and the regenerated code under
      `gen/discovery/v1` is included in the diff.
- [ ] **Config hygiene** — New configuration is env-var driven with a
      sensible default, not hardcoded. No secrets committed.
- [ ] **Test coverage** — New code paths are tested, including negative
      cases (errors, missing/stale data, empty inputs) alongside the
      happy path.
- [ ] **Error classification** — New domain errors are sentinel errors in
      `internal/app/errors.go` and are mapped in
      `internal/transport/grpc/handler.go`'s `classifyError`, not
      returned as raw/ungrouped errors across the transport boundary.

## Additional Notes

<!-- Anything else reviewers should know: follow-up work, migration steps, etc. -->
