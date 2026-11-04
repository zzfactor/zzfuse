FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS build
ARG TARGETOS TARGETARCH
WORKDIR /app
COPY pkg/ ./pkg/
COPY trazr-otel/ ./trazr-otel/
COPY trazrexporter/ ./trazrexporter/
COPY trazrextension/ ./trazrextension/
COPY trazrprocessor/ ./trazrprocessor/
COPY zzfuseauth/ ./zzfuseauth/
WORKDIR /app/trazr-otel
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath -o /out/otelcol-custom .
RUN mkdir -m 0700 -p /out/queue && chown 65532:65532 /out/queue

FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
WORKDIR /
COPY --from=build /out/otelcol-custom /otelcol-custom
COPY config.yaml /etc/otelcol/config.yaml
COPY --from=build --chown=65532:65532 /out/queue /var/lib/zzfuse/queue
USER nonroot:nonroot
ENTRYPOINT ["/otelcol-custom", "--feature-gates=+service.profilesSupport", "--config", "/etc/otelcol/config.yaml"]
