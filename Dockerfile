# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /bin/muninn ./cmd/muninn

FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source="https://github.com/Garoze/Muninn"
LABEL org.opencontainers.image.description="Kubernetes-native runtime configuration resolver"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=build /bin/muninn /muninn

EXPOSE 5010 5011 9090

ENTRYPOINT ["/muninn"]
