# Observability

Muninn emits three signals for every gRPC call and every admission request:
a distributed trace span, structured log entries, and metric observations.
See [`design.md`](design.md) for why each exists and what belongs in which.

## Metrics

Prometheus metrics are served on `$METRICS_ADDR` (default `:9090`) at
`/metrics`, in both modes. Labels are deliberately low-cardinality: operation
and outcome, never namespace.

## Tracing

Spans are exported over OTLP to `$OTEL_EXPORTER_OTLP_ENDPOINT` (default
`localhost:4317`). Nothing needs to be listening there for Muninn to run;
spans fail to export silently when the endpoint is unset or unreachable.

To view them locally, run [Jaeger](https://www.jaegertracing.io/)'s all-in-one
image, which bundles the collector, storage and UI in one container:

```bash
podman run -d --name jaeger \
  -p 16686:16686 -p 4317:4317 \
  docker.io/jaegertracing/all-in-one:latest
# or `docker run` in place of `podman run`: same image, same flags
```

> [!IMPORTANT]
> The default sample ratio is `0.1`, so a single manual query has only a 10%
> chance of being recorded. Set `OTEL_TRACES_SAMPLE_ARG=1` to sample every
> call for this walkthrough, or Jaeger may show nothing.

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_TRACES_SAMPLE_ARG=1
make run
```

```bash
make query NAMESPACE=arasaka KEYS=LOG_LEVEL
```

Open `http://localhost:16686`, select the `muninn` service, and find the trace
for that `Query` call.

Sampling is `ParentBased`, so a caller that already decided to trace a request
end to end keeps that decision here rather than having it re-rolled.

## Health

The resolver serves a gRPC health service on `$GRPC_PROBE_ADDR` (default
`:5011`), reporting `NOT_SERVING` until every registered source's informer
completes its initial list and watch.

The webhook serves `/healthz` and `/readyz` on its HTTPS port. Liveness only
proves the listener is up; readiness additionally requires a synced cache,
because an unsynced replica admits Pods without injecting into them.
