#!/usr/bin/env python3
"""Opt-in real zzfuse stack E2E. Requires project-built images, Go and Node.

Owns disposable containers, tmpfs PostgreSQL storage and an isolated network.
Public MiniLM weights and a pinned Chromium browser may be downloaded. No mocks,
host database volumes, production services, or external email recipients are used.
"""

import argparse
import base64
import copy
import json
import math
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
MODEL = "sentence-transformers/all-MiniLM-L6-v2"
PLAYWRIGHT_VERSION = "1.64.1"


def check(condition, message):
    if not condition:
        raise RuntimeError(message)


def run(*command, input=None, env=None, cwd=None, timeout=90, combined=False):
    process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, env=env, cwd=cwd,
                               start_new_session=True)
    try:
        stdout, stderr = process.communicate(input, timeout=timeout)
    except BaseException:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=5)
        raise
    if process.returncode:
        raise subprocess.CalledProcessError(process.returncode, command, stdout, stderr)
    return (stdout + stderr if combined else stdout).strip()


def request(url, *, data=None, headers=None, timeout=10, method=None):
    try:
        with urllib.request.urlopen(urllib.request.Request(url, data=data, headers=headers or {}, method=method),
                                    timeout=timeout) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def json_request(url, *, body=None, headers=None, timeout=10):
    combined = dict(headers or {})
    data = None
    if body is not None:
        combined["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    status, raw = request(url, data=data, headers=combined, timeout=timeout)
    if not raw:
        return status, None
    try:
        value = json.loads(raw)
    except json.JSONDecodeError:
        raise RuntimeError(f"Non-JSON response from {urllib.parse.urlsplit(url).path}: HTTP {status}") from None
    return status, value


def wait_for(predicate, description, *, timeout=90):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            value = predicate()
            if value:
                return value
        except (OSError, subprocess.SubprocessError) as error:
            last_error = error
        time.sleep(0.3)
    raise TimeoutError(f"Timed out waiting for {description}" +
                       (f": {type(last_error).__name__}" if last_error else ""))


def env_file(directory, name, values):
    target = directory / (name + ".env")
    target.write_text("".join(f"{key}={value}\n" for key, value in values.items()))
    target.chmod(0o600)
    return target


def tenant_security_checks(stack, api, otlp, sql, collector, database, mailpit, admin_auth,
                           payloads, profiles, tenant_keys, browser_env):
    """Exercise real login, grants, sessions, tenant writes and WAL crash recovery."""
    for priority, schema in enumerate(("tenant_a", "tenant_b"), start=10):
        status, response = json_request(api + "/config/schemas", headers=admin_auth, body={
            "name": schema, "enabled": True, "priority": priority,
            "config": {"database_schema": schema, "routing": {}},
        })
        check(status == 201, stack.redact(f"Tenant schema {schema} configuration failed: HTTP {status}; {json.dumps(response)}"))
    run("docker", "restart", collector, timeout=45)
    run("docker", "restart", mailpit, timeout=45)
    wait_for(lambda: request(api + "/health")[0] == 200, "Collector tenant restart")

    account = {"username": "tenant" + stack.owner, "email": "tenant-" + stack.owner + "@example.test",
               "password": secrets.token_hex(16)}
    stack.secrets.append(account["password"])
    status, registered = json_request(api + "/auth/register", body=account)
    check(status == 201, "Ordinary account registration failed")
    user_id = registered["id"]

    def login():
        status, session = json_request(api + "/auth/login", body=account)
        check(status == 200, "Ordinary account login failed")
        stack.secrets.append(session["token"])
        return {"Authorization": "Bearer " + session["token"]}

    user_auth = login()
    common = "&cursor=MHww&direction=asc&limit=100"
    check(request(api + "/logs?schema=otel" + common, headers=user_auth)[0] == 403,
          "Empty ordinary-user grants allowed telemetry access")
    check(json_request(api + "/schemas", headers=user_auth)[1]["schemas"] == [],
          "Schema discovery exposed other tenants")
    for path, body in (("schemas", {"user_id": user_id, "schemas": ["tenant_b"]}),
                       ("roles", {"user_id": user_id, "roles": ["admin"]})):
        check(json_request(api + "/admin/user/" + path, headers=user_auth, body=body)[0] == 403,
              "Ordinary user changed administrative access")
    check(json_request(api + "/admin/user/schemas", headers=admin_auth,
                       body={"user_id": user_id, "schemas": ["tenant_a"]})[0] == 200,
          "Administrator schema grant failed")
    for schema, key in zip(("tenant_a", "tenant_b"), tenant_keys):
        headers = {"Authorization": "Bearer " + key}
        for kind, payload in payloads.items():
            # Exact same service labels match the original default routing.
            # Only the credential is permitted to select the destination.
            check(json_request(otlp + "/v1/" + kind, body=payload, headers=headers)[0] == 200,
                  f"Credential-scoped {kind} ingestion failed")
        check(request(otlp + "/v1development/profiles", data=profiles,
                      headers={**headers, "Content-Type": "application/x-protobuf",
                               "OTLP-Profiles-Development-Version": "1"})[0] == 200,
              "Credential-scoped profile ingestion failed")
        wait_for(lambda schema=schema: sql(f"SELECT count(*) FROM {schema}.logs;") == "2", "Tenant logs")
        wait_for(lambda schema=schema: sql(f"SELECT count(*) FROM {schema}.spans;") == "1", "Tenant spans")
        for kind in ("gauge", "sum", "histogram", "summary", "exponential_histogram"):
            wait_for(lambda schema=schema, kind=kind: sql(f"SELECT count(*) FROM {schema}.{kind}_metrics;") == "1",
                     "Tenant metric routing")
        wait_for(lambda schema=schema: int(sql(f"SELECT count(*) FROM {schema}.profiles;")) > 0, "Tenant profiles")
    check(sql("SELECT count(*) FROM otel.logs;") == "2", "Tenant writes leaked into default schema")
    check(request(api + "/logs?schema=tenant_a" + common, headers=user_auth)[0] == 200,
          "Current grant did not apply to an already-issued user token")
    for path in ("logs", "spans", "profiles", "metrics?type=gauge"):
        separator = "&" if "?" in path else "?"
        check(request(api + "/" + path + separator + "schema=tenant_b" + common, headers=user_auth)[0] == 403,
              "Restricted user read another tenant")
    visible = json_request(api + "/schemas", headers=user_auth)[1]
    check([schema["schema"] for schema in visible["schemas"]] == ["tenant_a"], "Schema list ignored grants")
    check(browser_test(browser_env, {"mode": "tenant-stream", "api": api,
        "adminJwt": admin_auth["Authorization"][7:], "jwt": user_auth["Authorization"][7:],
        "userId": user_id}) == {"revocation": "passed"}, "Live WebSocket revocation failed")
    check(request(api + "/logs?schema=tenant_a" + common, headers=user_auth)[0] == 403,
          "Grant removal did not apply to existing JWT")
    check(json_request(api + "/admin/user/schemas", headers=admin_auth,
                       body={"user_id": user_id, "schemas": ["tenant_a"]})[0] == 200, "Grant restoration failed")
    disposable_session = login()
    check(request(api + "/auth/logout", data=b"", headers=disposable_session)[0] == 204, "Backend logout failed")
    check(request(api + "/auth/whoami", headers=disposable_session)[0] == 401, "Logout JWT remained usable")
    check(request(api + "/auth/renew", data=b"", headers=disposable_session)[0] == 401, "Logout JWT was renewable")
    check(request(api + "/auth/whoami", headers=user_auth)[0] == 200, "Logout revoked an unrelated session")
    check(json_request(api + "/admin/user/disabled", headers=admin_auth,
                       body={"user_id": user_id, "disabled": True})[0] == 204, "Account disable failed")
    check(request(api + "/auth/whoami", headers=user_auth)[0] == 401, "Disabled account JWT was usable")
    check(json_request(api + "/auth/login", body=account)[0] == 401, "Disabled account could log in")
    check(json_request(api + "/admin/user/disabled", headers=admin_auth,
                       body={"user_id": user_id, "disabled": False})[0] == 204, "Account enable failed")
    check(request(api + "/auth/whoami", headers=user_auth)[0] == 401, "Re-enable revived a revoked JWT")
    user_auth = login()
    check(json_request(api + "/admin/user/revoke-sessions", headers=admin_auth,
                       body={"user_id": user_id})[0] == 204, "Administrator session revocation failed")
    check(request(api + "/auth/whoami", headers=user_auth)[0] == 401, "Admin-revoked JWT was usable")
    user_auth = login()
    status, machine_key = json_request(api + "/auth/apikeys", headers=user_auth,
        body={"name": "read-only-e2e", "scopes": ["telemetry:read"], "expires_in": "1h"})
    check(status == 201, stack.redact(f"Scoped API key creation failed: HTTP {status}; {json.dumps(machine_key)}"))
    stack.secrets.append(machine_key["key"])
    status, exchanged = json_request(api + "/auth/apikeys/exchange",
        headers={"X-API-Key": machine_key["key"]}, body={})
    check(status == 200, "API key exchange failed")
    stack.secrets.append(exchanged["token"])
    machine_auth = {"Authorization": "Bearer " + exchanged["token"]}
    check(request(api + "/logs?schema=tenant_a" + common, headers=machine_auth)[0] == 200, "Read key could not read its tenant")
    check(json_request(api + "/logs?schema=tenant_a", headers=machine_auth, body=[])[0] == 403,
          "Read-only API key inserted telemetry")
    check(json_request(api + "/auth/apikeys", headers=machine_auth,
                       body={"name": "escalation", "scopes": ["telemetry:write"]})[0] == 403,
          "Machine JWT created a broader credential")
    check(request(api + "/auth/apikeys/" + machine_key["api_key"]["id"], headers=user_auth, method="DELETE")[0] == 204,
          "API key revocation failed")
    check(request(api + "/logs?schema=tenant_a" + common, headers=machine_auth)[0] == 401,
          "Revoked API key's JWT remained usable")
    status, write_key = json_request(api + "/auth/apikeys", headers=user_auth,
        body={"name": "write-only-e2e", "scopes": ["telemetry:write"], "expires_in": "1h"})
    check(status == 201, stack.redact(f"Write-only API key creation failed: HTTP {status}; {json.dumps(write_key)}"))
    stack.secrets.append(write_key["key"])
    status, write_token = json_request(api + "/auth/apikeys/exchange", headers={"X-API-Key": write_key["key"]}, body={})
    check(status == 200, "Write-only API key exchange failed")
    stack.secrets.append(write_token["token"])
    write_auth = {"Authorization": "Bearer " + write_token["token"]}
    check(request(api + "/metrics/gauge/1?schema=tenant_a", headers=write_auth, method="POST", data=b"{}")[0] == 403,
          "Write-only API key read a metric through POST")
    check(request(api + "/logs/cursor?schema=tenant_a", headers=write_auth, method="POST", data=b"{}")[0] == 403,
          "Write-only API key reached a cursor read handler")
    check(request(api + "/logs?schema=tenant_a", headers=write_auth, method="PUT", data=b"{}")[0] == 403,
          "Write-only API key used an unsupported mutation method")

    print("Pausing disposable PostgreSQL and forcing a Collector crash to check tenant WAL replay", flush=True)
    queued = copy.deepcopy(payloads["logs"])
    records = queued["resourceLogs"][0]["scopeLogs"][0]["logRecords"]
    records[:] = records[:1]
    records[0]["body"] = {"stringValue": "queued-crash-" + stack.owner}
    run("docker", "pause", database)
    try:
        for key in tenant_keys:
            check(json_request(otlp + "/v1/logs", body=queued,
                               headers={"Authorization": "Bearer " + key})[0] == 200, "WAL enqueue failed during DB outage")
        run("docker", "kill", "--signal", "KILL", collector)
    finally:
        run("docker", "unpause", database)
    run("docker", "start", collector)
    run("docker", "restart", mailpit, timeout=45)
    wait_for(lambda: request(api + "/health")[0] == 200, "Collector WAL replay readiness")
    for schema in ("tenant_a", "tenant_b"):
        wait_for(lambda schema=schema: sql(f"SELECT count(*) FROM {schema}.logs;") == "3", "Tenant-bound WAL replay")
    check(sql("SELECT count(*) FROM otel.logs;") == "2", "WAL replay lost tenant identity")
    grpc = json.loads(run("go", "run", "-mod=readonly", str(ROOT / "scripts/e2e/ingestion-grpc.go"),
        cwd=ROOT / "trazr-otel", input=json.dumps({"endpoint": "127.0.0.1:" + str(stack.port(collector, 4317)),
            "run_id": stack.owner, "tokens": dict(zip(("tenant_a", "tenant_b"), tenant_keys)),
            "profiles_file": str(ROOT / "pkg/repository/testdata/current-profiles.pb")}), timeout=120))
    check(grpc["status"] == "grpc_auth_passed", "gRPC authentication driver failed")
    service = grpc["service_name"]
    check(re.fullmatch(r"[a-zA-Z0-9_-]+", service), "Unexpected gRPC fixture service name")
    for schema in ("tenant_a", "tenant_b"):
        for table in ("logs", "spans", "gauge_metrics", "profiles"):
            wait_for(lambda schema=schema, table=table: sql(f"SELECT count(*) FROM {schema}.{table} WHERE service_name='{service}';") == "1",
                     "gRPC credential-bound signal persistence")
    check(sql(f"SELECT count(*) FROM otel.logs WHERE service_name='{service}';") == "0", "gRPC writes lost tenant isolation")


class OwnedStack:
    def __init__(self, directory, artifacts):
        self.owner = secrets.token_hex(8)
        self.prefix = "zzfuse-stack-" + self.owner
        self.network = self.prefix
        self.directory, self.artifacts = directory, artifacts
        self.containers = []
        self.network_created = False
        self.secrets = []

    def redact(self, value):
        for secret in self.secrets:
            value = value.replace(secret, "[test-secret]")
        return value

    def create_network(self):
        run("docker", "network", "create", "--label", f"zzfuse.e2e.owner={self.owner}", self.network)
        self.network_created = True

    def container(self, service, image, *, env=None, ports=(), options=(), command=(), network=None):
        name = self.prefix + "-" + service
        self.containers.append(name)  # A failed launch may still have created a container.
        args = ["docker", "run", "--detach", "--name", name, "--label", f"zzfuse.e2e.owner={self.owner}",
                "--network", network or self.network]
        if network is None:
            args += ["--network-alias", service]
        if env:
            args += ["--env-file", str(env_file(self.directory, service, env))]
        # Docker reassigns automatically published ports on restart. Allocate
        # explicit random host ports so crash/restart URLs remain stable.
        reservations = []
        try:
            for port in ports:
                reservation = socket.socket()
                reservation.bind(("127.0.0.1", 0))
                reservations.append(reservation)
                args += ["--publish", f"127.0.0.1:{reservation.getsockname()[1]}:{port}"]
        finally:
            for reservation in reservations:
                reservation.close()
        args += list(options) + [image] + list(command)
        run(*args, timeout=180)
        return name

    def port(self, name, internal):
        return int(run("docker", "port", name, f"{internal}/tcp").rsplit(":", 1)[1])

    def diagnostics(self):
        for name in self.containers:
            try:
                logs = run("docker", "logs", "--tail", "250", name, timeout=15, combined=True)
            except subprocess.CalledProcessError as error:
                logs = (error.stdout or "") + (error.stderr or "")
            except Exception as error:
                logs = type(error).__name__
            self.artifacts.joinpath(name + ".log").write_text(self.redact(logs))

    def cleanup(self):
        errors = []
        for name in reversed(self.containers):
            try:
                owner = run("docker", "inspect", "--format", '{{index .Config.Labels "zzfuse.e2e.owner"}}',
                            name, timeout=15)
                check(owner == self.owner, "Refusing to remove a container owned by another task")
                run("docker", "rm", "--force", "--volumes", name, timeout=40)
            except subprocess.CalledProcessError as error:
                if "No such" not in (error.stderr or ""):
                    errors.append(self.redact(error.stderr or str(error)))
            except Exception as error:
                errors.append(str(error))
        if self.network_created:
            try:
                run("docker", "network", "rm", self.network, timeout=30)
            except Exception as error:
                errors.append(str(error))
        return errors


class NATSObserver:
    """Small real NATS protocol subscriber; no broker/client mocks."""
    def __init__(self, port, token):
        self.socket = socket.create_connection(("127.0.0.1", port), timeout=5)
        self.socket.settimeout(0.5)
        self.buffer = b""
        self.messages = []
        self.error = None
        self.lock = threading.Lock()
        self.ready, self.stopped = threading.Event(), threading.Event()
        self.socket.sendall(("CONNECT " + json.dumps({"auth_token": token, "verbose": False,
                            "pedantic": True}) + "\r\nSUB > 1\r\nPING\r\n").encode())
        self.thread = threading.Thread(target=self.read, daemon=True)
        self.thread.start()
        check(self.ready.wait(10), "NATS subscription was not acknowledged")
        self.check_error()

    def read(self):
        try:
            while not self.stopped.is_set():
                try:
                    chunk = self.socket.recv(65536)
                except socket.timeout:
                    continue
                if not chunk:
                    raise RuntimeError("NATS connection closed")
                self.buffer += chunk
                while b"\r\n" in self.buffer:
                    line, rest = self.buffer.split(b"\r\n", 1)
                    if line.startswith(b"MSG "):
                        fields = line.split()
                        length = int(fields[-1])
                        if len(rest) < length + 2:
                            break
                        body = rest[:length]
                        check(rest[length:length + 2] == b"\r\n", "Invalid NATS frame")
                        self.buffer = rest[length + 2:]
                        subject = fields[1].decode()
                        if subject != "health_check":
                            with self.lock:
                                self.messages.append((subject, json.loads(body)))
                    else:
                        self.buffer = rest
                        if line == b"PING":
                            self.socket.sendall(b"PONG\r\n")
                        elif line == b"PONG":
                            self.ready.set()
                        elif line.startswith(b"-ERR"):
                            raise RuntimeError("NATS rejected subscription: " + line.decode())
        except BaseException as error:
            if not self.stopped.is_set():
                self.error = error
                self.ready.set()

    def check_error(self):
        if self.error:
            raise self.error

    def snapshot(self):
        self.check_error()
        with self.lock:
            return list(self.messages)

    def close(self):
        self.stopped.set()
        try:
            self.socket.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        self.socket.close()
        self.thread.join(timeout=3)


def normalized_profiles(payload):
    """Resolve the reachable graph so lossless compaction/reindexing stays legal."""
    dictionary = payload["dictionary"]
    strings = dictionary["stringTable"]
    references = {"mappingIndex": ("mapping", "mappingTable"),
                  "functionIndex": ("function", "functionTable"),
                  "stackIndex": ("stack", "stackTable"), "linkIndex": ("link", "linkTable")}

    def normalize(value):
        if isinstance(value, list):
            return [normalize(item) for item in value]
        if not isinstance(value, dict):
            return value
        result = {}
        for key, item in value.items():
            if key.endswith("Strindex"):
                result[key[:-8]] = strings[item]
            elif key == "attributeIndices":
                result["attributes"] = sorted((normalize(dictionary["attributeTable"][index]) for index in item),
                                              key=lambda entry: json.dumps(entry, sort_keys=True))
            elif key == "locationIndices":
                result["locations"] = [normalize(dictionary["locationTable"][index]) for index in item]
            elif key in references:
                label, table = references[key]
                result[label] = normalize(dictionary[table][item])
            elif key == "attributes" and isinstance(item, list):
                result[key] = sorted(normalize(item), key=lambda entry: json.dumps(entry, sort_keys=True))
            elif key == "kvlistValue":
                result[key] = normalize(item)
                result[key]["values"] = sorted(result[key].get("values", []),
                                               key=lambda entry: json.dumps(entry, sort_keys=True))
            else:
                result[key] = normalize(item)
        return result

    result = {}
    for resource in payload["resourceProfiles"]:
        for scope in resource["scopeProfiles"]:
            for profile in scope["profiles"]:
                key = profile["profileId"]
                check(key not in result, "Duplicate fixture profile identity")
                result[key] = {"resource": normalize(resource.get("resource", {})),
                               "resourceSchema": resource.get("schemaUrl", ""),
                               "scope": normalize(scope.get("scope", {})),
                               "scopeSchema": scope.get("schemaUrl", ""), "profile": normalize(profile)}
    return result


def telemetry_fixture(run_id):
    # Trace end_time uses PostgreSQL timestamp precision; keep this fixture
    # aligned so its 1 ms duration is exactly representable in that schema.
    now = time.time_ns() // 1_000_000 * 1_000_000
    trace_id, span_id = secrets.token_hex(16), secrets.token_hex(8)
    service = "zzfuse-stack-e2e"
    texts = [f"zzfuse checkout completed for {run_id}", f"zzfuse payment accepted for {run_id}"]
    attributes = [{"key": "service.name", "value": {"stringValue": service}},
                  {"key": "service.namespace", "value": {"stringValue": "stack-e2e"}},
                  {"key": "service.instance.id", "value": {"stringValue": run_id}}]
    resource = {"attributes": attributes}
    scope = {"name": "zzfuse.stack-e2e", "version": "1"}
    marker = [{"key": "e2e.run_id", "value": {"stringValue": run_id}}]
    logs = {"resourceLogs": [{"resource": resource, "scopeLogs": [{"scope": scope, "logRecords": [
        {"timeUnixNano": str(now + index), "observedTimeUnixNano": str(now + index),
         "severityNumber": 9, "severityText": "INFO", "body": {"stringValue": text},
         "traceId": trace_id, "spanId": span_id, "attributes": marker}
        for index, text in enumerate(texts)]}]}]}
    traces = {"resourceSpans": [{"resource": resource, "scopeSpans": [{"scope": scope, "spans": [
        {"traceId": trace_id, "spanId": span_id, "name": "stack-e2e-checkout", "kind": 2,
         "startTimeUnixNano": str(now), "endTimeUnixNano": str(now + 1000000),
         "attributes": marker, "status": {"code": 1}}]}]}]}
    point = {"timeUnixNano": str(now), "startTimeUnixNano": str(now - 1000000), "attributes": marker}
    metrics = [
        {"name": "stack_e2e_gauge", "unit": "items", "gauge": {"dataPoints": [dict(point, asDouble=7.25)]}},
        {"name": "stack_e2e_sum", "sum": {"aggregationTemporality": 2, "isMonotonic": True,
                                           "dataPoints": [dict(point, asInt="5")]}},
        {"name": "stack_e2e_histogram", "histogram": {"aggregationTemporality": 2, "dataPoints": [
            dict(point, count="2", sum=4.0, bucketCounts=["1", "1"], explicitBounds=[2.0])]}},
        {"name": "stack_e2e_summary", "summary": {"dataPoints": [
            dict(point, count="2", sum=4.0, quantileValues=[{"quantile": 0.5, "value": 2.0}])]}},
        {"name": "stack_e2e_exponential_histogram", "exponentialHistogram": {
            "aggregationTemporality": 2, "dataPoints": [dict(point, count="2", sum=4.0, scale=0,
                zeroCount="0", positive={"offset": 0, "bucketCounts": ["1", "1"]})]}},
    ]
    return {"logs": logs, "traces": traces, "metrics": {"resourceMetrics": [{"resource": resource,
            "scopeMetrics": [{"scope": scope, "metrics": metrics}]}]}}, texts, trace_id, span_id, service


def rfc3339_unix_ns(value):
    seconds, nanos = divmod(int(value), 1_000_000_000)
    fraction = f"{nanos:09d}".rstrip("0")
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(seconds)) + ("." + fraction if fraction else "") + "Z"


def check_metric_measurement(kind, metric):
    if kind == "gauge":
        check(metric["value"] == 7.25, "Gauge measurement changed")
    elif kind == "sum":
        check(metric["value"] == 5, "Sum measurement changed")
    else:
        check(metric["count"] == 2 and metric["sum"] == 4.0, "Metric count/sum changed")
        if kind == "histogram":
            check(metric["buckets"] == {"bucket_counts": [1, 1], "explicit_bounds": [2.0]},
                  "Histogram distribution changed")
        elif kind == "summary":
            check(metric["quantiles"] == [{"quantile": 0.5, "value": 2.0}], "Summary quantiles changed")
        else:
            check(metric["scale"] == 0 and metric["zero_count"] == 0 and
                  metric["positive_offset"] == 0 and metric["positive_bucket_counts"] == [1, 1],
                  "Exponential histogram distribution changed")


def prepare_browser(directory):
    directory.mkdir(parents=True, exist_ok=True)
    package = directory / "node_modules/playwright/package.json"
    if not package.is_file() or json.loads(package.read_text())["version"] != PLAYWRIGHT_VERSION:
        print(f"Installing isolated Playwright {PLAYWRIGHT_VERSION}", flush=True)
        run("npm", "install", "--prefix", str(directory), "--no-audit", "--no-fund", "--ignore-scripts",
            "playwright@" + PLAYWRIGHT_VERSION, timeout=600)
    env = dict(os.environ, PLAYWRIGHT_BROWSERS_PATH=str(directory / "browsers"),
               ZZFUSE_PLAYWRIGHT_DIR=str(directory))
    executable = run("node", "-e", "console.log(require(process.env.ZZFUSE_PLAYWRIGHT_DIR+"
                     "'/node_modules/playwright').chromium.executablePath())", env=env)
    if not Path(executable).is_file():
        print("Downloading isolated Chromium browser", flush=True)
        run("node", str(directory / "node_modules/playwright/cli.js"), "install", "chromium", env=env, timeout=600)
    return env


def browser_test(env, payload):
    return json.loads(run("node", str(ROOT / "scripts/e2e/browser.cjs"), input=json.dumps(payload),
                          env=env, timeout=150))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database-image", default="zzfuse-database:local")
    parser.add_argument("--collector-image", default="zzfuse-collector:release-readiness")
    parser.add_argument("--services-image", default="zzfuse-services:release-readiness")
    parser.add_argument("--frontend-image", default="zzfuse-frontend:release-readiness")
    parser.add_argument("--nats-image", default="nats:2.15.1-alpine@sha256:2ae57dfba3063d5e25fecb75256118c0882a1a2d2cf7d0fba17a7c701df956f6")
    parser.add_argument("--mailpit-image", default="axllent/mailpit:v1.31.5@sha256:94f6b70c1436defae6799d9ac7ff0eeff9540465b842026d5cdb018f1c0234ea")
    parser.add_argument("--playwright-dir", type=Path, default=ROOT / ".local/e2e-tools/playwright-1.64.1")
    parser.add_argument("--artifacts-dir", type=Path, default=ROOT / ".local/e2e-output")
    args = parser.parse_args()
    for image in (args.database_image, args.collector_image, args.services_image, args.frontend_image):
        try:
            run("docker", "image", "inspect", image)
        except subprocess.CalledProcessError:
            parser.error(f"Project image {image} is missing; build project images first")
    browser_env = prepare_browser(args.playwright_dir.resolve())
    for image in (args.nats_image, args.mailpit_image):
        run("docker", "pull", image, timeout=300)

    with tempfile.TemporaryDirectory(prefix="zzfuse-stack-e2e-") as temporary:
        directory = Path(temporary)
        artifacts = args.artifacts_dir.resolve() / (time.strftime("%Y%m%dT%H%M%S") + "-" + secrets.token_hex(4))
        artifacts.mkdir(parents=True)
        stack = OwnedStack(directory, artifacts)
        observer = None
        completed = False
        password, jwt_secret, next_secret, user_key, admin_key, nats_token, ingestion_key, receipt_key, tenant_a_key, tenant_b_key, api_key_pepper = [secrets.token_hex(24) for _ in range(11)]
        stack.secrets.extend([password, jwt_secret, next_secret, user_key, admin_key, nats_token, ingestion_key, receipt_key, tenant_a_key, tenant_b_key, api_key_pepper])
        try:
            stack.create_network()
            print("Starting disposable project database and real NATS/Python/frontend", flush=True)
            database = stack.container("database", args.database_image, ports=[5432],
                env={"POSTGRES_DB": "zzfuse_stack_e2e", "POSTGRES_USER": "postgres", "POSTGRES_PASSWORD": password},
                options=["--tmpfs", "/var/lib/postgresql/data:rw,size=1g"])
            db_port = stack.port(database, 5432)

            def sql(statement):
                return run("docker", "exec", "--interactive", database, "sh", "-c",
                    'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -h127.0.0.1 -U postgres '
                    '-d zzfuse_stack_e2e -At -v ON_ERROR_STOP=1', input=statement, timeout=20)

            wait_for(lambda: sql("SELECT 1;") == "1", "PostgreSQL TCP readiness")
            versions = sql("SELECT extname || '=' || extversion FROM pg_extension "
                           "WHERE extname IN ('timescaledb','vector','vectorscale') ORDER BY extname;")
            check(len(versions.splitlines()) == 3, "Project DB lacks required extensions")
            nats_config = directory / "nats.conf"
            nats_config.write_text('port: 4222\nhttp_port: 8222\nauthorization { token: $NATS_TOKEN }\n')
            nats = stack.container("nats", args.nats_image, ports=[4222, 8222], env={"NATS_TOKEN": nats_token},
                options=["--mount", f"type=bind,source={nats_config},target=/etc/nats/e2e.conf,readonly"],
                command=["--config", "/etc/nats/e2e.conf"])
            nats_port = stack.port(nats, 4222)
            wait_for(lambda: request(f"http://127.0.0.1:{stack.port(nats, 8222)}/healthz")[0] == 200, "NATS")
            observer = NATSObserver(nats_port, nats_token)
            services = stack.container("services", args.services_image, ports=[8000],
                env={"USER_API_KEY": user_key, "ADMIN_API_KEY": admin_key, "DEFAULT_MODEL_NAME": MODEL,
                     "DEVICE": "cpu", "GUNICORN_WORKERS": "1", "MAX_MODELS_IN_CACHE": "1",
                     "OMP_NUM_THREADS": "2", "TOKENIZERS_PARALLELISM": "false"})
            services_port = stack.port(services, 8000)
            embedding_url = f"http://127.0.0.1:{services_port}"
            embedding_headers = {"X-API-Key": user_key}
            wait_for(lambda: request(embedding_url + "/sentencer/health", headers=embedding_headers)[0] == 200,
                     "Python embedding API", timeout=120)
            check(request(embedding_url + "/sentencer/health")[0] == 401, "Embedding API accepted missing key")
            status, embedded = json_request(embedding_url + "/sentencer/embed", timeout=120,
                headers=embedding_headers, body={"texts": ["real zzfuse stack inference"], "model_name": MODEL})
            check(status == 200 and embedded["model_name"] == MODEL, "Real MiniLM inference failed")
            vector = embedded["embeddings"][0]
            check(len(vector) == 384 and all(math.isfinite(value) for value in vector), "Invalid model vector")
            check(sum(value * value for value in vector) > 0.1, "Model emitted an empty vector")

            with socket.socket() as reservation:
                reservation.bind(("127.0.0.1", 0))
                frontend_port = reservation.getsockname()[1]
            frontend_url = f"http://127.0.0.1:{frontend_port}"
            frontend = stack.container("frontend", args.frontend_image,
                env={"BACKEND_URL": "http://collector:18888", "NEXTAUTH_SECRET": next_secret,
                     "NEXTAUTH_URL": frontend_url, "HOSTNAME": "0.0.0.0", "PORT": "3000"},
                options=["--publish", f"127.0.0.1:{frontend_port}:3000"])
            wait_for(lambda: request(frontend_url + "/login")[0] == 200, "Frontend", timeout=120)

            config = directory / "collector.yaml"
            pipelines = "".join(f"    {kind}:\n      receivers: [otlp]\n      exporters: [trazr]\n"
                                for kind in ["logs", "traces", "metrics", "profiles"])
            config.write_text("extensions:\n  trazr:\n    endpoint: 0.0.0.0:18888\n"
                "  zzfuse_auth:\n    signing_key: ${env:TRAZR_INGESTION_SIGNING_KEY}\n    credentials:\n"
                "      default:\n        token: ${env:TRAZR_INGESTION_TOKEN}\n        schema: otel\n        epoch: 1\n"
                "      tenant_a:\n        token: ${env:ZZFUSE_TEST_TENANT_A_TOKEN}\n        schema: tenant_a\n        epoch: 1\n"
                "      tenant_b:\n        token: ${env:ZZFUSE_TEST_TENANT_B_TOKEN}\n        schema: tenant_b\n        epoch: 1\n"
                "  file_storage/ingestion:\n    directory: /var/lib/zzfuse/queue\n    create_directory: true\n    directory_permissions: \"0700\"\n    fsync: true\nreceivers:\n"
                "  otlp:\n    protocols:\n      grpc:\n        endpoint: 0.0.0.0:4317\n        auth:\n          authenticator: zzfuse_auth\n      http:\n        endpoint: 0.0.0.0:4318\n        auth:\n          authenticator: zzfuse_auth\nexporters:\n"
                "  trazr:\n    authenticator: zzfuse_auth\n    sending_queue:\n      enabled: true\n      queue_size: 100\n      num_consumers: 1\n      storage: file_storage/ingestion\n"
                "    retry_on_failure:\n      enabled: true\n      max_elapsed_time: 0s\nservice:\n  extensions: [trazr, zzfuse_auth, file_storage/ingestion]\n"
                "  telemetry:\n    logs:\n      level: warn\n    metrics:\n      level: none\n  pipelines:\n" + pipelines)
            collector = stack.container("collector", args.collector_image, ports=[18888, 4317, 4318, 8025],
                env={"TRAZR_DB_HOST": "database", "TRAZR_DB_PORT": "5432", "TRAZR_DB_USER": "postgres",
                     "TRAZR_DB_PASSWORD": password, "TRAZR_DB_NAME": "zzfuse_stack_e2e", "TRAZR_DB_SSL_MODE": "disable",
                     "TRAZR_JWT_SECRET": jwt_secret, "EVENT_BUS_TYPE": "nats", "NATS_URL": f"nats://{nats_token}@nats:4222",
                     "TRAZR_API_KEY_PEPPER": api_key_pepper,
                     "TRAZR_INGESTION_TOKEN": ingestion_key, "TRAZR_INGESTION_SIGNING_KEY": receipt_key,
                     "ZZFUSE_TEST_TENANT_A_TOKEN": tenant_a_key, "ZZFUSE_TEST_TENANT_B_TOKEN": tenant_b_key,
                     "EMBEDDING_API_KEY": user_key, "TRAZR_SMTP_HOST": "127.0.0.1", "TRAZR_SMTP_PORT": "1025",
                     "TRAZR_SMTP_FROM": "noreply@example.test", "TRAZR_SMTP_TLS_MODE": "plain",
                     "TRAZR_PASSWORD_RESET_URL": frontend_url + "/reset-password"},
                options=["--mount", f"type=bind,source={config},target=/etc/otelcol/e2e.yaml,readonly",
                         "--entrypoint", "/otelcol-custom"],
                command=["--feature-gates=+service.profilesSupport", "--config", "/etc/otelcol/e2e.yaml"])
            api = f"http://127.0.0.1:{stack.port(collector, 18888)}/api/v1"
            otlp = f"http://127.0.0.1:{stack.port(collector, 4318)}"
            mailpit = stack.container("mailpit", args.mailpit_image, network="container:" + collector,
                            env={"MP_SMTP_BIND_ADDR": "127.0.0.1:1025", "MP_UI_BIND_ADDR": "0.0.0.0:8025"})
            mailbox = f"http://127.0.0.1:{stack.port(collector, 8025)}"
            wait_for(lambda: request(mailbox + "/api/v1/messages")[0] == 200, "Local SMTP capture")
            wait_for(lambda: request(api + "/health")[0] == 200, "Collector", timeout=120)
            print("Real model inference ready; checking Chromium registration/login", flush=True)
            username, email, user_password = "e2e" + stack.owner, stack.owner + "@example.test", secrets.token_hex(16)
            stack.secrets.append(user_password)
            session = browser_test(browser_env, {"mode": "register-login", "url": frontend_url,
                "username": username, "email": email, "password": user_password,
                "screenshot": str(artifacts / "frontend-login.png")})
            stack.secrets.append(session["jwt"])
            auth = {"Authorization": "Bearer " + session["jwt"]}
            status, identity = json_request(api + "/auth/whoami", headers=auth)
            check(status == 200, "Browser NextAuth token was not accepted by backend")

            print("Ingesting real logs/traces/all metric types/profiles through OTLP", flush=True)
            payloads, texts, trace_id, span_id, service_name = telemetry_fixture(stack.owner)
            ingestion_auth = {"Authorization": "Bearer " + ingestion_key}
            for kind, payload in payloads.items():
                check(request(otlp + "/v1/" + kind, data=json.dumps(payload).encode(),
                              headers={"Content-Type": "application/json"})[0] == 401,
                      f"OTLP {kind} accepted missing credentials")
                status, response = json_request(otlp + "/v1/" + kind, body=payload, headers=ingestion_auth)
                check(status == 200 and not response.get("partialSuccess"), f"OTLP {kind} ingestion failed: HTTP {status}")
            fixture_dir = ROOT / "pkg/repository/testdata"
            expected_profiles = normalized_profiles(json.loads((fixture_dir / "current-profiles.json").read_text()))
            status, raw = request(otlp + "/v1development/profiles", data=(fixture_dir / "current-profiles.pb").read_bytes(),
                                 headers={**ingestion_auth, "Content-Type": "application/x-protobuf", "OTLP-Profiles-Development-Version": "1"})
            check(status == 200, "OTLP profiles ingestion failed")
            wait_for(lambda: sql("SELECT count(*) FROM otel.logs;") == "2", "Stored logs")
            wait_for(lambda: sql("SELECT count(*) FROM otel.spans;") == "1", "Stored spans")
            wait_for(lambda: int(sql("SELECT count(*) FROM otel.profiles;")) == len(expected_profiles), "Stored profiles")
            common = "schema=otel&cursor=MHww&direction=asc&limit=100"
            for path in ["logs", "spans", "profiles", "metrics?type=gauge"]:
                separator = "&" if "?" in path else "?"
                check(request(api + "/" + path + separator + common)[0] == 401, "Unauthenticated telemetry query succeeded")
            status, logs = json_request(api + "/logs?" + common, headers=auth)
            check(status == 200 and sorted(row["body"] for row in logs["data"]) == sorted(texts), "Log body roundtrip failed")
            check(all(row["trace_id"] == trace_id and row["span_id"] == span_id for row in logs["data"]), "Log correlation IDs changed")
            for row in logs["data"]:
                check(row["resource_attributes"]["service.instance.id"] == stack.owner and
                      row["scope_name"] == "zzfuse.stack-e2e", "Log resource/scope context changed")
            status, spans = json_request(api + "/spans?" + common, headers=auth)
            check(status == 200 and len(spans["data"]) == 1,
                  f"Trace roundtrip failed: HTTP {status}; {json.dumps(spans) if status != 200 else 'incorrect row count'}")
            check(spans["data"][0]["trace_id"] == trace_id and spans["data"][0]["span_id"] == span_id, "Span identities changed")
            span = spans["data"][0]
            expected_span = payloads["traces"]["resourceSpans"][0]["scopeSpans"][0]["spans"][0]
            check(span["start_time"] == rfc3339_unix_ns(expected_span["startTimeUnixNano"]) and
                  span["end_time"] == rfc3339_unix_ns(expected_span["endTimeUnixNano"]) and
                  span["duration_ns"] == 1_000_000, "Span time/duration changed")
            check(span["name"] == expected_span["name"] and span["service_name"] == service_name and
                  span["resource_attributes"]["service.instance.id"] == stack.owner and
                  span["scope_name"] == "zzfuse.stack-e2e", "Span name/resource/scope context changed")
            for kind in ["gauge", "sum", "histogram", "summary", "exponential_histogram"]:
                table = kind + "_metrics"
                wait_for(lambda table=table: sql(f"SELECT count(*) FROM otel.{table};") == "1", "Stored " + kind)
                status, metrics = json_request(api + "/metrics?type=" + kind + "&" + common, headers=auth)
                check(status == 200 and len(metrics["data"]) == 1,
                      f"Metric query failed for {kind}: HTTP {status}; {json.dumps(metrics) if status != 200 else 'incorrect row count'}")
                check(metrics["data"][0]["name"] == "stack_e2e_" + kind, "Metric identity changed")
                metric = metrics["data"][0]
                check(metric["resource_attributes"]["service.instance.id"] == stack.owner and
                      metric["scope_name"] == "zzfuse.stack-e2e", "Metric resource/scope context changed")
                check_metric_measurement(kind, metric)
            status, profiles = json_request(api + "/profiles?" + common, headers=auth)
            check(status == 200 and len(profiles["data"]) == len(expected_profiles), "Profiles query failed")
            for row in profiles["data"]:
                check(row["profile_decode_status"] == "decoded", "Stored profile decoding failed")
                normalized = normalized_profiles(row["profile_data"])
                check(len(normalized) == 1, "Profile row was not isolated")
                profile_id = row["otel_profile_id"]
                check(normalized[profile_id] == expected_profiles[profile_id], "Reachable profile graph/context changed")
                check(row["profile_time_unix_nano"] == expected_profiles[profile_id]["profile"]["timeUnixNano"], "Exact profile time changed")
                check(row["duration_nano"] == expected_profiles[profile_id]["profile"]["durationNano"], "Exact profile duration changed")
                persisted = sql(f"SELECT encode(profile_bytes,'hex') FROM otel.profiles WHERE id={int(row['profile_id'])};")
                check(bytes.fromhex(persisted) == base64.b64decode(row["profile_bytes"], validate=True), "API profile bytes differ from DB")

            required_subjects = {"log.inserted", "trace.inserted", "profile.inserted"} | {
                "metric." + kind + ".inserted" for kind in ["gauge", "sum", "histogram", "summary", "exponential_histogram"]}
            wait_for(lambda: required_subjects.issubset({subject for subject, _ in observer.snapshot()}), "Real NATS insertion events")
            event_log_bodies, event_profile_ids = set(), set()
            for subject, event in observer.snapshot():
                if subject in required_subjects:
                    check(event["type"] == subject and event["schema"] == "otel" and event["data"], "Invalid NATS event contract")
                    if subject == "log.inserted":
                        bodies = {row["body"] for row in event["data"]}
                        check(bodies.issubset(set(texts)), "NATS log body changed")
                        event_log_bodies.update(bodies)
                    elif subject == "trace.inserted":
                        check(all(row["trace_id"] == trace_id and row["span_id"] == span_id and
                                  row["duration_ns"] == 1_000_000 for row in event["data"]), "NATS span identities/duration changed")
                    elif subject.startswith("metric."):
                        for row in event["data"]:
                            check_metric_measurement(subject.split(".")[1], row)
                    elif subject == "profile.inserted":
                        for row in event["data"]:
                            graph = normalized_profiles(row["profile_data"])
                            check(len(graph) == 1 and graph[row["otel_profile_id"]] == expected_profiles[row["otel_profile_id"]],
                                  "NATS profile graph/context changed")
                            event_profile_ids.add(row["otel_profile_id"])
            check(event_log_bodies == set(texts) and event_profile_ids == set(expected_profiles),
                  "NATS insertion events omitted fixture rows")

            print("Starting production manager workers and checking persisted pgvector inference", flush=True)
            driver_env = {key: value for key, value in os.environ.items()
                          if not key.startswith(("TRAZR_", "NATS_", "EVENT_BUS_", "EMBEDDING_"))}
            driver_env.update(TRAZR_DB_HOST="127.0.0.1", TRAZR_DB_PORT=str(db_port), TRAZR_DB_USER="postgres",
                TRAZR_DB_PASSWORD=password, TRAZR_DB_NAME="zzfuse_stack_e2e", TRAZR_DB_SSL_MODE="disable",
                EVENT_BUS_TYPE="nats", NATS_URL=f"nats://{nats_token}@127.0.0.1:{nats_port}",
                EMBEDDING_API_KEY=user_key, ZZFUSE_E2E_EMBEDDING_URL=embedding_url,
                ZZFUSE_E2E_QUERY=texts[0], ZZFUSE_E2E_EXPECTED_LOGS="2", ZZFUSE_E2E_ALLOW_DISPOSABLE_DB="true")
            driver = run("go", "run", "-mod=readonly", str(ROOT / "scripts/e2e/embedding-worker.go"),
                         cwd=ROOT / "pkg/repository", env=driver_env, timeout=240)
            artifacts.joinpath("worker-driver.log").write_text(stack.redact(driver))
            check("ZZFUSE_WORKER_E2E_PASS" in driver, "Worker driver did not confirm success")

            print("Checking tenant grants, sessions, live streams and crash recovery", flush=True)
            tenant_security_checks(stack, api, otlp, sql, collector, database, mailpit, auth, payloads,
                (fixture_dir / "current-profiles.pb").read_bytes(), (tenant_a_key, tenant_b_key), browser_env)

            print("Checking real local SMTP reset and browser password change", flush=True)
            current_password = user_password
            received_message_ids = set()
            for iteration, signed_in in enumerate((False, True), start=1):
                status, prior_session = json_request(api + "/auth/login", body={"username": email, "password": current_password})
                check(status == 200, "Pre-reset session login failed")
                stack.secrets.append(prior_session["token"])
                status, known = json_request(frontend_url + "/api/reset-password", body={"email": email})
                status_missing, missing = json_request(frontend_url + "/api/reset-password", body={"email": "missing-" + email})
                check(status == status_missing == 200 and known == missing and
                      known == {"status": "if the account exists, a password reset email will be sent"},
                      "Password reset response leaks account/token or failed")
                def new_reset_message():
                    status, listing = json_request(mailbox + "/api/v1/messages")
                    check(status == 200, "SMTP mailbox listing failed")
                    messages = listing.get("messages", [])
                    fresh = [message for message in messages if message["ID"] not in received_message_ids]
                    return (messages, fresh) if fresh else None

                messages, fresh = wait_for(new_reset_message, "New private SMTP reset email")
                check(len(messages) == iteration and len(fresh) == 1, "Unknown account unexpectedly received reset email")
                received_message_ids.add(fresh[0]["ID"])
                status, message = json_request(mailbox + "/api/v1/message/" + fresh[0]["ID"])
                check(status == 200 and any(recipient["Address"] == email for recipient in message["To"]), "Reset email recipient incorrect")
                urls = re.findall(r'https?://[^\s<>"\']+', message.get("Text", "") + " " + message.get("HTML", ""))
                reset_url = next((url for url in urls if urllib.parse.urlsplit(url).path == "/reset-password"), None)
                check(reset_url is not None and reset_url.startswith(frontend_url + "/reset-password#"), "Email reset link origin/path incorrect")
                reset_token = urllib.parse.parse_qs(urllib.parse.urlsplit(reset_url).fragment).get("token", [""])[0]
                check(bool(reset_token), "Email has no private reset token")
                stack.secrets.append(reset_token)
                new_password = secrets.token_hex(16)
                stack.secrets.append(new_password)
                browser_test(browser_env, {"mode": "reset-login", "url": frontend_url, "resetUrl": reset_url,
                    "email": email, "password": new_password, "oldPassword": current_password, "signedIn": signed_in,
                    "screenshot": str(artifacts / ("frontend-reset-signed-in.png" if signed_in else "frontend-reset.png"))})
                check(json_request(api + "/auth/reset-password", body={"token": reset_token, "new_password": new_password})[0] == 400,
                      "Consumed reset token was replayable")
                check(json_request(api + "/auth/login", body={"username": email, "password": current_password})[0] == 401,
                      "Old password remained valid")
                check(json_request(api + "/auth/login", body={"username": email, "password": new_password})[0] == 200,
                      "New password was not accepted")
                check(request(api + "/auth/whoami", headers={"Authorization": "Bearer " + prior_session["token"]})[0] == 401,
                      "Password reset left a previously-issued backend JWT usable")
                current_password = new_password
            check(len(json_request(mailbox + "/api/v1/messages")[1].get("messages", [])) == 2,
                  "Unknown account unexpectedly received reset email")
            logged_out = browser_test(browser_env, {"mode": "logout", "url": frontend_url, "email": email,
                "password": current_password, "screenshot": str(artifacts / "frontend-logout.png")})
            stack.secrets.append(logged_out["jwt"])
            check(request(api + "/auth/whoami", headers={"Authorization": "Bearer " + logged_out["jwt"]})[0] == 401,
                  "Browser logout did not revoke its backend session")
            summary = {"database_extensions": versions.splitlines(), "model": MODEL, "embedding_dimension": len(vector),
                       "logs": 2, "spans": 1, "metric_types": 5, "profiles": len(expected_profiles),
                       "nats_subjects": sorted(required_subjects), "browser_auth": "passed", "smtp_reset": "passed",
                       "worker_inference": "passed"}
            summary.update(tenant_isolation="passed", session_revocation="passed",
                           websocket_revocation="passed", tenant_wal_crash_recovery="passed")
            artifacts.joinpath("result.json").write_text(json.dumps(summary, indent=2) + "\n")
            completed = True
        except subprocess.CalledProcessError as error:
            print(stack.redact((error.stdout or "") + (error.stderr or "")), file=sys.stderr)
            raise RuntimeError("External stack command failed; see captured diagnostics") from None
        finally:
            active_error = sys.exc_info()[0] is not None
            cleanup_errors = []
            try:
                if observer is not None:
                    try:
                        observer.close()
                    except Exception as error:
                        cleanup_errors.append("NATS observer: " + type(error).__name__)
                try:
                    stack.diagnostics()
                except Exception as error:
                    cleanup_errors.append("Diagnostic capture: " + type(error).__name__)
            finally:
                cleanup_errors.extend(stack.cleanup())
            if cleanup_errors:
                print("Owned resource cleanup failed: " + "; ".join(cleanup_errors), file=sys.stderr)
                if not active_error:
                    raise RuntimeError("Full-stack cleanup failed")
            print(f"Artifacts: {artifacts}", flush=True)
        check(completed, "Full-stack tests did not complete")
        print("PASS: real project DB, Collector, NATS, MiniLM inference, pgvector workers, Chromium auth and local SMTP reset", flush=True)


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: (_ for _ in ()).throw(KeyboardInterrupt()))
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(130)
