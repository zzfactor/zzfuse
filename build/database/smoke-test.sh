#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${1:-zzfuse-database:local}
container="zzfuse-db-smoke-$(date +%s)-$$"
task_dir=$(mktemp -d "${TMPDIR:-/tmp}/zzfuse-db-smoke.XXXXXX")
cleanup() {
    docker rm -f "$container" >/dev/null 2>&1 || true
    rm -rf "$task_dir"
}
trap cleanup EXIT HUP INT TERM

# No published ports, host bind mounts, or persistent volumes.
docker run -d --name "$container" \
    --tmpfs /var/lib/postgresql/data:rw,nosuid,noexec,size=512m \
    --shm-size=128m --memory=1g \
    -e POSTGRES_USER=zzfuse_smoke \
    -e POSTGRES_PASSWORD=isolated-smoke-only \
    -e POSTGRES_DB=zzfuse_smoke "$image" >/dev/null

attempt=0
until docker exec "$container" pg_isready -h 127.0.0.1 -U zzfuse_smoke -d zzfuse_smoke >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 60 ]; then
        docker logs "$container"
        echo 'Database did not become ready within 60 seconds' >&2
        exit 1
    fi
    sleep 1
done

if ! docker exec -i "$container" psql -X -U zzfuse_smoke -d zzfuse_smoke < "$script_dir/smoke.sql" > "$task_dir/sql.log" 2>&1; then
    cat "$task_dir/sql.log"
    docker logs "$container"
    exit 1
fi
cat "$task_dir/sql.log"

# Extension initialization is idempotent, and PostgreSQL dropped root privileges.
docker exec -i "$container" psql -X -v ON_ERROR_STOP=1 -U zzfuse_smoke -d zzfuse_smoke < "$script_dir/init/020_zzfuse_extensions.sql" >/dev/null
test "$(docker exec "$container" ps -o user= -p 1 | tr -d ' ')" = postgres
echo 'Fresh initialization, SQL functionality, and server ownership passed.'
