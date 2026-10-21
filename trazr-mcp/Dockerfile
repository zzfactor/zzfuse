# Build stage
FROM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS builder
WORKDIR /app/trazr-mcp
# Build from the repository root so the local auth/client replacement is present.
COPY trazr-mcp/ ./
COPY pkg/auth/client/ ../pkg/auth/client/
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/trazr-mcp .

FROM gcr.io/distroless/static:nonroot@sha256:516757106de35205efb70b91ba03f6f1298424a0678c603255cf2852a7000087
COPY --from=builder /out/trazr-mcp /trazr-mcp
ENTRYPOINT ["/trazr-mcp"]
