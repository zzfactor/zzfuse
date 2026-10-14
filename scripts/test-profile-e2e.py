#!/usr/bin/env python3
"""Opt-in profiles E2E against an owned, disposable TimescaleDB container."""

import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def profile_semantics(node, dictionary):
    """Expand OTLP dictionary references independently of their numeric indexes."""
    if isinstance(node, list):
        return [profile_semantics(value, dictionary) for value in node]
    if not isinstance(node, dict):
        return node
    tables = {
        "attributeIndices": ("attributes", "attributeTable", True),
        "locationIndices": ("locations", "locationTable", True),
        "stackIndex": ("stack", "stackTable", False),
        "mappingIndex": ("mapping", "mappingTable", False),
        "functionIndex": ("function", "functionTable", False),
        "linkIndex": ("link", "linkTable", False),
    }
    result = {}
    for key, value in node.items():
        if key.endswith("Strindex"):
            result[key[:-8]] = dictionary["stringTable"][int(value)]
        elif key in tables:
            label, table, repeated = tables[key]
            if repeated:
                result[label] = [profile_semantics(dictionary[table][int(index)], dictionary)
                                 for index in value]
            else:
                result[label] = profile_semantics(dictionary[table][int(value)], dictionary)
        else:
            result[key] = profile_semantics(value, dictionary)
    return result


def run(*args, input=None):
    return subprocess.run(args, input=input, text=True, check=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout.strip()


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def request(url, *, data=None, headers=None):
    try:
        with urllib.request.urlopen(urllib.request.Request(url, data=data, headers=headers or {}),
                                    timeout=5) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def wait_for(predicate, description, process=None):
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        if process is not None and process.poll() is not None:
            raise RuntimeError(f"Collector exited while waiting for {description}")
        try:
            if predicate():
                return
        except (OSError, subprocess.CalledProcessError):
            pass
        time.sleep(0.25)
    raise TimeoutError(f"Timed out waiting for {description}")


def stop_collector(process):
    if process is not None and process.poll() is None:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)


def main():
    if not __debug__:
        raise RuntimeError("Profile checks require Python assertions; unset PYTHONOPTIMIZE and omit -O")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--collector", type=Path, default=ROOT / "trazr-otel/trazr-otel")
    parser.add_argument("--image", default="zzfuse-database:local")
    parser.add_argument("--disable-queue", action="store_true",
                        help="Exercise synchronous export instead of the default sending queue")
    args = parser.parse_args()
    collector = args.collector.resolve()
    if not collector.is_file():
        parser.error("Collector binary is missing; run make build first")
    fixture_dir = ROOT / "pkg/repository/testdata"
    current = (fixture_dir / "current-profiles.pb").read_bytes()
    current_row = (fixture_dir / "current-profile-row0.pb").read_bytes()
    expected = json.loads((fixture_dir / "current-profiles.json").read_text())
    legacy = (fixture_dir / "profiles-0.135/legacy-0.135-profile-row.pb").read_bytes()
    name = "zzfuse-profile-e2e-" + secrets.token_hex(6)
    password, jwt_secret, ingestion_token, signing_key, user_password = [secrets.token_hex(32) for _ in range(5)]
    process = None
    created = False
    with tempfile.TemporaryDirectory(prefix="zzfuse-profile-e2e-") as temporary:
        directory = Path(temporary)
        (directory / "queue").mkdir()
        log_path = directory / "collector.log"
        try:
            run("docker", "run", "--detach", "--rm", "--name", name,
                "--label", "zzfuse.test=profiles", "--publish", "127.0.0.1::5432",
                "--env", "POSTGRES_DB=zzfuse_profile_e2e", "--env", "POSTGRES_USER=postgres",
                "--env", f"POSTGRES_PASSWORD={password}", args.image)
            created = True
            db_port = run("docker", "port", name, "5432/tcp").rsplit(":", 1)[1]

            def sql(statement):
                return run("docker", "exec", "--interactive", name, "sh", "-c",
                           'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -h 127.0.0.1 -U postgres '
                           '-d zzfuse_profile_e2e -At -v ON_ERROR_STOP=1', input=statement)

            def database_ready():
                # Docker's port proxy can accept TCP before PostgreSQL is ready.
                # The SQL probe must use TCP too; Unix sockets reach the temporary
                # initialization server that shuts down before normal startup.
                with socket.create_connection(("127.0.0.1", int(db_port)), timeout=1):
                    return sql("SELECT 1;") == "1"

            wait_for(database_ready, "PostgreSQL")
            try:
                sql("CREATE EXTENSION IF NOT EXISTS timescaledb;")
            except subprocess.CalledProcessError as error:
                # Image initialization can race extension setup. IF NOT EXISTS
                # cannot serialize concurrent installers.
                if "pg_extension_name_index" not in error.stderr or "(timescaledb)" not in error.stderr:
                    raise
                wait_for(lambda: sql("SELECT EXISTS (SELECT 1 FROM pg_extension "
                                     "WHERE extname='timescaledb');") == "t",
                         "TimescaleDB image initialization")
            api_port, otlp_port = free_port(), free_port()
            while otlp_port == api_port:
                otlp_port = free_port()
            config = directory / "collector.yaml"
            queue_enabled = "false" if args.disable_queue else "true"
            config.write_text(f"""extensions:
  trazr:
    endpoint: 127.0.0.1:{api_port}
  zzfuse_auth:
    signing_key: ${{env:TRAZR_INGESTION_SIGNING_KEY}}
    credentials:
      default:
        token: ${{env:TRAZR_INGESTION_TOKEN}}
        schema: otel
        epoch: 1
  file_storage/ingestion:
    directory: {directory / "queue"}
    create_directory: true
    directory_permissions: "0700"
    fsync: true
receivers:
  otlp:
    protocols:
      http:
        endpoint: 127.0.0.1:{otlp_port}
        auth:
          authenticator: zzfuse_auth
exporters:
  trazr:
    authenticator: zzfuse_auth
    sending_queue:
      enabled: {queue_enabled}
      queue_size: 100
      num_consumers: 1
      storage: file_storage/ingestion
    retry_on_failure:
      enabled: false
service:
  extensions: [trazr, zzfuse_auth, file_storage/ingestion]
  telemetry:
    logs:
      level: warn
    metrics:
      level: none
  pipelines:
    profiles:
      receivers: [otlp]
      exporters: [trazr]
""")
            env = {key: value for key, value in os.environ.items()
                   if not key.startswith(("TRAZR_", "OTEL_", "EVENT_BUS_", "NATS_"))}
            env.update(TRAZR_DB_HOST="127.0.0.1", TRAZR_DB_PORT=db_port,
                       TRAZR_DB_USER="postgres", TRAZR_DB_PASSWORD=password,
                       TRAZR_DB_NAME="zzfuse_profile_e2e", TRAZR_DB_SSL_MODE="disable",
                       TRAZR_JWT_SECRET=jwt_secret, TRAZR_INGESTION_TOKEN=ingestion_token,
                       TRAZR_INGESTION_SIGNING_KEY=signing_key, EVENT_BUS_TYPE="in-memory")
            api = f"http://127.0.0.1:{api_port}/api/v1"
            # Begin pagination at epoch/ID zero to include fixed historical fixtures.
            profiles_url = api + "/profiles?schema=otel&cursor=MHww&direction=asc&limit=100"

            with log_path.open("w") as log:
                def start():
                    nonlocal process
                    process = subprocess.Popen([str(collector),
                        "--feature-gates=+service.profilesSupport", "--config", str(config)],
                        cwd=directory, env=env, stdout=log, stderr=subprocess.STDOUT)
                    wait_for(lambda: request(api + "/health")[0] == 200,
                             "Collector HTTP readiness", process)
                    return process

                process = start()
                assert request(profiles_url)[0] == 401, "Profiles query must require authentication"
                account = {"username": "profile-test", "email": "profile-test@example.test", "password": user_password}
                status, body = request(api + "/auth/register", data=json.dumps(account).encode(),
                                       headers={"Content-Type": "application/json"})
                assert status == 201, "Real profile test account registration failed"
                status, body = request(api + "/auth/login", data=json.dumps(account).encode(),
                                       headers={"Content-Type": "application/json"})
                assert status == 200, "Real profile test account login failed"
                auth = {"Authorization": "Bearer " + json.loads(body)["token"]}
                headers = {"Content-Type": "application/x-protobuf",
                           "OTLP-Profiles-Development-Version": "1", "Authorization": "Bearer " + ingestion_token}
                otlp = f"http://127.0.0.1:{otlp_port}/v1development/profiles"
                wait_for(lambda: request(otlp, data=b"", headers=headers)[0] == 200,
                         "OTLP profile readiness", process)
                status, body = request(otlp, data=current, headers=headers)
                assert status == 200, f"OTLP ingestion failed ({status}): {body!r}"
                expected_count = sum(len(scope.get("profiles", []))
                    for resource in expected["resourceProfiles"]
                    for scope in resource["scopeProfiles"])
                wait_for(lambda: int(sql("SELECT count(*) FROM otel.profiles;")) == expected_count,
                         "persisted profiles", process)
                status, body = request(profiles_url, headers=auth)
                assert status == 200, f"Profile query failed ({status}): {body!r}"
                rows = json.loads(body)["data"]
                assert len(rows) == expected_count, "All ingested profiles must be queryable"
                assert len({row["otel_profile_id"] for row in rows}) == expected_count
                expected_profiles = {
                    profile["profileId"]: (resource, scope, profile)
                    for resource in expected["resourceProfiles"]
                    for scope in resource["scopeProfiles"]
                    for profile in scope["profiles"]
                }
                for row in rows:
                    assert row["profile_storage_version"] == 1
                    assert row["profile_encoding"] == "otlp-profiles-protobuf"
                    assert row["profile_schema_version"] == "1"
                    assert base64.b64decode(row["profile_bytes"], validate=True)
                    decoded = row["profile_data"]
                    assert decoded["dictionary"], "Each stored profile needs its complete dictionary"
                    resources = decoded["resourceProfiles"]
                    assert len(resources) == 1 and len(resources[0]["scopeProfiles"]) == 1
                    profiles = resources[0]["scopeProfiles"][0]["profiles"]
                    assert len(profiles) == 1, "Per-row payload must contain exactly one profile"
                    profile = profiles[0]
                    assert str(profile["timeUnixNano"]) == row["profile_time_unix_nano"]
                    assert str(profile["durationNano"]) == row["duration_nano"]
                    assert row["profile_decode_status"] == "decoded"
                    assert profile.get("samples"), "Profile samples must survive ingestion"
                    original_resource, original_scope, original_profile = expected_profiles[row["otel_profile_id"]]
                    actual_semantics = profile_semantics(profile, decoded["dictionary"])
                    expected_semantics = profile_semantics(original_profile, expected["dictionary"])
                    assert actual_semantics == expected_semantics, "Profile graph fields or samples changed"
                    assert profile_semantics(resources[0].get("resource"), decoded["dictionary"]) == \
                        profile_semantics(original_resource.get("resource"), expected["dictionary"])
                    assert resources[0].get("schemaUrl") == original_resource.get("schemaUrl")
                    stored_scope = resources[0]["scopeProfiles"][0]
                    assert profile_semantics(stored_scope.get("scope"), decoded["dictionary"]) == \
                        profile_semantics(original_scope.get("scope"), expected["dictionary"])
                    assert stored_scope.get("schemaUrl") == original_scope.get("schemaUrl")
                    stored_hex = sql("SELECT encode(profile_bytes, 'hex') FROM otel.profiles "
                                     f"WHERE id = {int(row['profile_id'])};")
                    assert bytes.fromhex(stored_hex) == base64.b64decode(row["profile_bytes"])

                bad_headers = dict(headers, **{"OTLP-Profiles-Development-Version": "999"})
                assert request(otlp, data=current, headers=bad_headers)[0] == 400
                legacy_dto = {"timestamp": "2026-10-14T00:00:00Z", "service_name": "legacy-e2e",
                              "profile_type": "cpu", "profile_bytes": base64.b64encode(legacy).decode()}
                json_headers = dict(auth, **{"Content-Type": "application/json"})
                modern_dto = dict(legacy_dto, service_name="forged-summary",
                    profile_bytes=base64.b64encode(current_row).decode(), profile_storage_version=1,
                    profile_encoding="otlp-profiles-protobuf", profile_schema_version="1")
                status, body = request(api + "/profiles?schema=otel",
                    data=json.dumps(modern_dto).encode(), headers=json_headers)
                assert status == 200, f"Modern REST insert failed ({status}): {body!r}"
                inserted_id = int(json.loads(body)["ids"][0])
                status, body = request(profiles_url, headers=auth)
                assert status == 200
                inserted = next(row for row in json.loads(body)["data"] if row["profile_id"] == inserted_id)
                assert base64.b64decode(inserted["profile_bytes"]) == current_row
                assert inserted["service_name"] == "profile-review", "Summaries must derive from protobuf"
                assert inserted["profile_decode_status"] == "decoded"
                status, body = request(api + "/profiles?schema=otel",
                    data=json.dumps(legacy_dto).encode(), headers=json_headers)
                assert status == 200, f"Legacy insert failed ({status}): {body!r}"
                invalid = dict(legacy_dto, profile_storage_version=1,
                    profile_encoding="otlp-profiles-protobuf", profile_schema_version="1")
                assert request(api + "/profiles?schema=otel", data=json.dumps(invalid).encode(),
                               headers=json_headers)[0] == 400, "Old wire must not be labeled current"
                assert int(sql("SELECT count(*) FROM otel.profiles;")) == expected_count + 2
                stop_collector(process)
                process = start()  # Reapply startup migration and reuse existing rows.
                status, body = request(profiles_url, headers=auth)
                assert status == 200
                rows = json.loads(body)["data"]
                assert len(rows) == expected_count + 2
                old = next(row for row in rows if row["service_name"] == "legacy-e2e")
                assert old["profile_storage_version"] == 0
                assert base64.b64decode(old["profile_bytes"]) == legacy, "Legacy bytes changed"
                assert old.get("profile_data") is None, "Unknown legacy protobuf must stay opaque"
                stored_hex = sql("SELECT encode(profile_bytes, 'hex') FROM otel.profiles "
                                 "WHERE service_name = 'legacy-e2e';")
                assert bytes.fromhex(stored_hex) == legacy
                result = (f"PASS: {expected_count} current profiles ingested through OTLP, queried with auth, "
                          "modern REST roundtrip, legacy bytes preserved, bad versions rejected, "
                          "restart migration idempotent")
        except Exception as error:
            if isinstance(error, subprocess.CalledProcessError):
                print(error.stderr)
            if log_path.exists():
                diagnostic = log_path.read_text()[-12000:]
                for secret in (password, jwt_secret, ingestion_token, signing_key, user_password):
                    diagnostic = diagnostic.replace(secret, "[test-secret]")
                print(diagnostic)
            raise
        finally:
            failed = sys.exc_info()[0] is not None
            cleanup_errors = []
            try:
                stop_collector(process)
            except Exception as error:
                cleanup_errors.append(str(error))
            finally:
                if created:
                    try:
                        run("docker", "stop", "--time", "5", name)
                    except Exception as error:
                        cleanup_errors.append(str(error))
            if cleanup_errors:
                print("Owned test resources could not be fully stopped: " + "; ".join(cleanup_errors),
                      file=sys.stderr)
                if not failed:
                    raise RuntimeError("E2E cleanup failed")
        print(result)


if __name__ == "__main__":
    main()
