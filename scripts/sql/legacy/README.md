# Legacy SQL helpers

These scripts preserve earlier one-off schema and trigger repairs. They are not the current migration or initialization path and are not applied by the build/check targets. Inspect them against a disposable database before reuse. Current DDL lives in `pkg/repository/internal/database/ddl/`.
