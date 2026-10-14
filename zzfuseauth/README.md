# Tenant-authenticated OTLP ingestion

`zzfuse_auth` implements the Collector server authenticator for HTTP and gRPC OTLP. Producers send `Authorization: Bearer <ingestion-token>`. These are independent random ingestion secrets, not user JWTs or account API keys. Generate each ingestion token and the separate receipt-signing key with `openssl rand -hex 32` and keep them in deployment secrets.

The operator maps each credential ID to one existing telemetry schema. Credential IDs and schemas use lowercase SQL-style names, at most 63 characters; reserved administration/system schemas are rejected. Tokens must be unique and at least 32 bytes, and each credential requires an epoch greater than zero. Missing configuration prevents startup. Missing, duplicate, malformed, and wrong authorization values fail authentication.

```yaml
extensions:
  zzfuse_auth:
    signing_key: ${env:TRAZR_INGESTION_SIGNING_KEY}
    credentials:
      tenant_a:
        token: ${env:TENANT_A_INGESTION_TOKEN}
        schema: tenant_a
        epoch: 1
      tenant_b:
        token: ${env:TENANT_B_INGESTION_TOKEN}
        schema: tenant_b
        epoch: 1
  file_storage/ingestion:
    directory: /var/lib/zzfuse/queue
    create_directory: true
    directory_permissions: "0700"
    fsync: true
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        auth:
          authenticator: zzfuse_auth
      http:
        endpoint: 0.0.0.0:4318
        auth:
          authenticator: zzfuse_auth
exporters:
  trazr:
    authenticator: zzfuse_auth
    sending_queue:
      enabled: true
      storage: file_storage/ingestion
      queue_size: 10000
      num_consumers: 1
service:
  extensions: [trazr, zzfuse_auth, file_storage/ingestion]
```

The example shows the authentication and queue sections; use the maintained root config for complete pipelines and the `trazr` service extension. Provision each configured telemetry schema before ingestion. Producer `service.name`, `service.namespace`, custom tenant headers, and resource attributes are labels, not authorization or routing authority. Logs, traces, all metric types, and profiles always write to the authenticated credential's schema. A single request cannot select multiple destination schemas. Profile dictionaries remain isolated per stored row.

The authenticator discards incoming client metadata and replaces it with a signed `x-zzfuse-ingestion-receipt`. The receipt contains only credential ID, schema and epoch. API tokens and signing keys are not added to telemetry or queued client metadata. Before enqueueing, the exporter requires this authenticator's private `client.Auth` marker and a matching receipt. A copied receipt cannot authorize fresh ingestion. At delivery, the exporter verifies the receipt. The Collector's standard persistent request encoding preserves client metadata, while dropping `client.Auth`; this receipt therefore retains authenticated identity across queue replay without persisting the API token. Unauthenticated old queue entries are rejected rather than routed using resource labels.

Keep the same signing key, credential ID, schema and epoch when restarting a Collector to drain its queue. Rotating the ingestion token with those fields unchanged rejects the old token for new requests and preserves admitted queued work. Remove the credential, change its schema, or increment its epoch to revoke its pending receipts. Rotating the signing key invalidates all old receipts. Invalid receipts are permanent export errors and their queued data will be discarded; plan revocation and key rotation accordingly.

The supported pipelines omit processors that discard client context, including `batch`. Exporter queue batching is rejected by validation because it can merge tenant identities. A pipeline that discards or forges the receipt fails closed. Protect the persistent volume as sensitive telemetry storage; Compose mounts an owned queue volume and the image provides a directory owned by its nonroot runtime user. Fsync is enabled and supported configs retry transient export failures without a time limit. The queue is finite, disk failures can prevent admission, and delivery is not exactly once across database writes and queue acknowledgement.

Compose exposes ingestion only on loopback. For remote producers, configure TLS on both OTLP protocols or use a trusted TLS terminator with a private Collector connection; bearer credentials must not cross untrusted networks in plaintext. Never log incoming authorization headers, dump deployment secret environment variables, or enable plaintext public ingestion for convenience.

Unit and race checks run from this module and `trazrexporter`. The opt-in full-stack harness checks actual HTTP/gRPC authentication, tenant-separated persisted telemetry, and queue replay after a Collector restart. The standalone gRPC driver is `scripts/e2e/ingestion-grpc.go`; it accepts only a disposable loopback endpoint and emits no credentials.

Collector contracts: [authentication configuration](https://opentelemetry.io/docs/collector/configuration/#authentication), [persistent file storage](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.163.0/extension/storage/filestorage/README.md), and [request-context encoding](https://github.com/open-telemetry/opentelemetry-collector/blob/v0.163.0/pdata/xpdata/request/context.go).
