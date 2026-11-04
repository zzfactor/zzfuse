# Build stage
FROM golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS builder
WORKDIR /app/trazr-mcp
# Build from the repository root so the local auth/client replacement is present.
COPY trazr-mcp/ ./
COPY pkg/auth/client/ ../pkg/auth/client/
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/trazr-mcp .

FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=builder /out/trazr-mcp /trazr-mcp
ENTRYPOINT ["/trazr-mcp"]
