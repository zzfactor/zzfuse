FROM --platform=$BUILDPLATFORM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS build
ARG TARGETOS TARGETARCH
WORKDIR /app
COPY pkg/ ./pkg/
COPY trazr-otel/ ./trazr-otel/
COPY trazrexporter/ ./trazrexporter/
COPY trazrextension/ ./trazrextension/
COPY trazrprocessor/ ./trazrprocessor/
WORKDIR /app/trazr-otel
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath -o /out/otelcol-custom .

FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /
COPY --from=build /out/otelcol-custom /otelcol-custom
COPY config.yaml /etc/otelcol/config.yaml
USER nonroot:nonroot
ENTRYPOINT ["/otelcol-custom", "--feature-gates=+service.profilesSupport", "--config", "/etc/otelcol/config.yaml"]
