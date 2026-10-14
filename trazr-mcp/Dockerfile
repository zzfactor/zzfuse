# Build stage
FROM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS builder
WORKDIR /app/trazr-mcp
# Build from the repository root so the local auth/client replacement is present.
COPY trazr-mcp/ ./
COPY pkg/auth/client/ ../pkg/auth/client/
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/trazr-mcp .

FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=builder /out/trazr-mcp /trazr-mcp
ENTRYPOINT ["/trazr-mcp"]
