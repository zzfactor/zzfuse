# Legacy exporter integration fixture

[Preserved source](legacy-exporter-integration-test.go.txt) formerly lived at `trazrexporter/integration_test.go`. It targets exporter-owned database clients, the removed `Config.schemaName` helper, and obsolete or misspelled exporter types. It cannot compile against the extension-owned service-manager architecture and is excluded from supported Go test builds.

Current exporter lifecycle unit tests cover Collector-host provider discovery, readiness, shared-manager borrowing, initialization failures, and shutdown ownership. A future disposable-database fixture must start the extension and pass its service provider through the Collector host, then verify exporter writes through the shared manager. See [current tasks](../../TASKS.md).
