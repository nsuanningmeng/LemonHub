#!/usr/bin/env python3
"""Read-only checks before replacing an existing Compose app using MySQL 8.2.

Run in the original deployment directory. Docker inspection/configuration can
contain credentials: keep it in memory and never echo raw CLI errors or output.
This does not stop containers, back up data, migrate, or approve an upgrade.
"""

import argparse
import json
import re
import subprocess
import sys
from urllib.parse import unquote


class PreflightError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise PreflightError(message)


def docker(args, description):
    try:
        result = subprocess.run(
            ["docker", *args], capture_output=True, text=True, check=False, timeout=45
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise PreflightError(description + " failed; no raw output is shown.") from exc
    require(result.returncode == 0, description + " failed; no raw output is shown.")
    return result.stdout


def docker_json(args, description):
    try:
        return json.loads(docker(args, description))
    except (ValueError, TypeError) as exc:
        raise PreflightError(description + " did not return valid JSON.") from exc


def env_map(container):
    return dict(item.split("=", 1) for item in container["Config"].get("Env", []) if "=" in item)


def mysql_target(dsn):
    # Credentials may contain @, /, or parentheses. Parse only the final network
    # and database suffix; do not log the DSN or accept SQLite/default fallback.
    match = re.fullmatch(r".*@tcp\(([^)]+)\)/([^?]+)(?:\?.*)?", dsn, flags=re.DOTALL)
    require(match is not None, "SQL_DSN must explicitly use a MySQL TCP target.")
    address, database = match.groups()
    require(":" in address, "MySQL TCP target must include an explicit port.")
    host, port = address.rsplit(":", 1)
    require(port == "3306", "This preflight requires the MySQL container's internal port 3306.")
    return host, unquote(database)


def duration_seconds(value):
    # Compose config emits Go duration strings (for example 2m10s).
    text = str(value)
    parts = re.findall(r"(\d+(?:\.\d+)?)(h|ms|us|ns|m|s)", text)
    require(parts and "".join(number + unit for number, unit in parts) == text,
            "stop_grace_period must be an explicit positive duration.")
    units = {"h": 3600, "m": 60, "s": 1, "ms": .001, "us": .000001, "ns": .000000001}
    return sum(float(number) * units[unit] for number, unit in parts)


def configured_mount(config, mount):
    kind, source = mount.get("type"), mount.get("source")
    require(kind in ("bind", "volume") and source,
            "Anonymous or nonpersistent mounts require manual review.")
    if kind == "volume":
        source = config.get("volumes", {}).get(source, {}).get("name")
        require(source, "The resolved name of a persistent volume is missing.")
    else:
        require(source.startswith("/"), "Bind sources must resolve to absolute paths.")
    return kind, source, mount.get("target"), not mount.get("read_only", False)


def actual_mount(mount):
    source = mount.get("Name") if mount.get("Type") == "volume" else mount.get("Source")
    return mount.get("Type"), source, mount.get("Destination"), mount.get("RW", False)


def network_names(config, service):
    names = set()
    for key in service.get("networks", {}):
        name = config.get("networks", {}).get(key, {}).get("name")
        require(name, "A resolved Compose network name is missing.")
        names.add(name)
    require(names, "Host/none/custom network mode requires manual review.")
    return names


def check_configuration(config, app, mysql, args):
    services = config.get("services", {})
    require(args.app_service in services and args.mysql_service in services,
            "Both selected services must exist in the resolved Compose configuration.")
    project = config.get("name")
    require(project, "Compose must return its resolved project name.")
    require(app.get("Id") != mysql.get("Id"), "Application and MySQL must be different containers.")
    for container, service in ((app, args.app_service), (mysql, args.mysql_service)):
        labels = container["Config"].get("Labels", {}) or {}
        require(labels.get("com.docker.compose.project") == project and
                labels.get("com.docker.compose.service") == service,
                "Existing container project/service does not match this Compose configuration.")
        require(not labels.get("com.docker.compose.oneoff", "false").lower() == "true",
                "A one-off container cannot establish the deployment identity.")
    require(mysql.get("State", {}).get("Running"), "The existing MySQL container must be running.")
    require(app.get("State", {}).get("Status") in ("running", "exited"),
            "The existing application must be running or cleanly stopped.")
    if app["State"]["Status"] == "exited":
        require(app["State"].get("ExitCode") == 0 and not app["State"].get("OOMKilled"),
                "The application did not stop cleanly; reconcile writes before upgrading.")

    planned_app, planned_mysql = services[args.app_service], services[args.mysql_service]
    host_config = app.get("HostConfig", {})
    for planned_key, current_key in (("extra_hosts", "ExtraHosts"), ("dns", "Dns"),
                                     ("dns_search", "DnsSearch"), ("dns_opt", "DnsOptions")):
        require(not planned_app.get(planned_key) and not host_config.get(current_key),
                "Custom application host/DNS routing requires manual review; it can redirect an unchanged SQL_DSN.")
    require(not planned_app.get("links") and not planned_app.get("external_links") and not host_config.get("Links"),
            "Legacy container links require manual review; their aliases can redirect an unchanged SQL_DSN.")
    image = planned_app.get("image", "")
    pinned = bool(re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", image))
    versioned = bool(re.fullmatch(r"[^\s]+:v?\d+\.\d+\.\d+(?:[-.]\d+)?", image))
    require(pinned or versioned, "Pin the application image to a release version or sha256 digest.")
    require(not planned_app.get("build"), "Use an already validated release image, not an implicit build.")
    require(planned_mysql.get("image") == mysql["Config"].get("Image"),
            "The MySQL image reference changed; an engine upgrade is outside this procedure.")

    current_env, target_env = env_map(app), planned_app.get("environment", {})
    require(isinstance(target_env, dict), "Compose must resolve environment entries to a map.")
    require(current_env.get("SQL_DSN") and target_env.get("SQL_DSN"),
            "SQL_DSN is missing; refuse a possible fallback to an empty SQLite database.")
    for key in ("SQL_DSN", "LOG_SQL_DSN", "REDIS_CONN_STRING", "SESSION_SECRET", "CRYPTO_SECRET"):
        require(current_env.get(key, "") == (target_env.get(key) or ""),
                key + " changed; preserve the existing value during the image-only upgrade.")
    host, database = mysql_target(target_env["SQL_DSN"])
    require(database == args.database, "SQL_DSN selects a different database than the explicit original name.")

    mysql_networks = mysql.get("NetworkSettings", {}).get("Networks", {})
    app_networks = app.get("NetworkSettings", {}).get("Networks", {})
    shared = set(mysql_networks) & set(app_networks)
    require(shared, "The existing application and MySQL have no shared Docker network.")
    aliases = {alias for network in shared for alias in mysql_networks[network].get("Aliases", []) or []}
    aliases.add(mysql.get("Name", "").lstrip("/"))
    require(host in aliases, "SQL_DSN does not select an alias of the explicitly selected MySQL container.")
    require(not any(host in (network or {}).get("aliases", [])
                    for network in planned_app.get("networks", {}).values()),
            "The application must not advertise the MySQL hostname as its own network alias.")
    for service, current in ((planned_app, app_networks), (planned_mysql, mysql_networks)):
        require(network_names(config, service) == set(current),
                "Docker network identity changed; preserve the original networks.")

    current_mounts = mysql.get("Mounts", [])
    require(not any(m.get("Destination", "").startswith("/var/lib/mysql/") for m in current_mounts),
            "Nested MySQL data mounts require manual review.")
    data_mounts = [m for m in current_mounts if m.get("Destination") == "/var/lib/mysql"]
    require(len(data_mounts) == 1, "MySQL must have exactly one persistent /var/lib/mysql mount.")
    identity = actual_mount(data_mounts[0])
    require(identity[0] in ("bind", "volume") and identity[3],
            "MySQL data must be on a writable bind mount or named volume.")
    require(identity[1] == args.expected_mysql_source,
            "The actual MySQL source differs from the explicitly recorded original mount.")
    planned_data = [m for m in planned_mysql.get("volumes", []) if m.get("target") == "/var/lib/mysql"]
    require(len(planned_data) == 1 and configured_mount(config, planned_data[0]) == identity,
            "Compose would use a different MySQL mount; refuse a possible empty database.")
    require(not any(m.get("target", "").startswith("/var/lib/mysql/")
                    for m in planned_mysql.get("volumes", [])), "Nested MySQL data mounts require manual review.")
    require(sorted(configured_mount(config, m) for m in planned_app.get("volumes", [])) ==
            sorted(actual_mount(m) for m in app.get("Mounts", [])),
            "Application persistent mounts changed; preserve /data, logs and other original mounts.")
    try:
        timeout = int(target_env.get("SHUTDOWN_TIMEOUT_SECONDS") or "120")
    except ValueError as exc:
        raise PreflightError("SHUTDOWN_TIMEOUT_SECONDS must be a positive integer.") from exc
    require(timeout > 0 and duration_seconds(planned_app.get("stop_grace_period", "0s")) > timeout,
            "stop_grace_period must exceed SHUTDOWN_TIMEOUT_SECONDS (default 120s).")

    databases = [database]
    log_dsn = target_env.get("LOG_SQL_DSN")
    if log_dsn:
        log_host, log_database = mysql_target(log_dsn)
        require(log_host == host, "A separate log database server needs its own preservation/backup review.")
        if log_database not in databases:
            databases.append(log_database)
    return {"project": project, "app_service": args.app_service, "mysql_service": args.mysql_service,
            "app_container_id": app.get("Id"), "mysql_container_id": mysql.get("Id"),
            "target_app_image": image, "current_app_image_id": app.get("Image"),
            "mysql_image_id": mysql.get("Image"), "mysql_mount_type": identity[0],
            "mysql_mount_source": identity[1], "databases": databases,
            "session_secret_configured": bool(target_env.get("SESSION_SECRET"))}


def inspect_database(container, database, primary):
    # The query and shell are constant. Database selection is an environment
    # argument, never interpolated into SQL or shell code. Passwords stay inside
    # the existing container, and raw mysql errors are suppressed by docker().
    query = """set -eu
if [ -n "${MYSQL_ROOT_PASSWORD_FILE:-}" ]; then
  MYSQL_PWD="$(cat "$MYSQL_ROOT_PASSWORD_FILE")"
else
  MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-}"
fi
export MYSQL_PWD
exec mysql --default-character-set=utf8mb4 -uroot --batch --skip-column-names \
  --database="$PREFLIGHT_DATABASE" -e "$PREFLIGHT_QUERY"
"""
    sql = ("SELECT @@version, @@server_uuid, @@datadir, "
           "(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE())")
    if primary:
        sql += ", (SELECT COUNT(*) FROM users), (SELECT COUNT(*) FROM tokens), (SELECT COUNT(*) FROM options)"
    output = docker(["exec", "--env", "PREFLIGHT_DATABASE=" + database,
                     "--env", "PREFLIGHT_QUERY=" + sql, container, "sh", "-c", query],
                    "Read-only MySQL identity/schema query")
    fields = output.strip().split("\t")
    require(len(fields) == (7 if primary else 4) and fields[0].startswith("8.2."),
            "The selected server is not a verified MySQL 8.2 instance.")
    require(re.fullmatch(r"[0-9a-fA-F-]{36}", fields[1]), "MySQL returned an invalid server UUID.")
    require(fields[2].rstrip("/") == "/var/lib/mysql",
            "MySQL uses a nonstandard data directory; verify its persistent mount manually.")
    require(fields[3].isdigit() and int(fields[3]) > 0, "The selected database has no tables.")
    if primary:
        require(all(v.isdigit() for v in fields[4:]) and int(fields[4]) > 0,
                "The selected primary database has no existing users or expected business tables.")
    return {"database": database, "server_version": fields[0], "server_uuid": fields[1], "table_count": int(fields[3])}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app-container", required=True, help="Explicit existing application container name/ID")
    parser.add_argument("--mysql-container", required=True, help="Explicit existing MySQL container name/ID")
    parser.add_argument("--database", required=True, help="Original primary database name (no credentials)")
    parser.add_argument("--expected-mysql-source", required=True, help="Recorded original named volume or absolute bind path")
    parser.add_argument("--app-service", default="new-api")
    parser.add_argument("--mysql-service", default="mysql")
    parser.add_argument("--file", action="append", required=True, help="Original Compose file, repeat in original order")
    parser.add_argument("--project-name", required=True, help="Original Compose project name")
    parser.add_argument("--env-file", action="append", default=[], help="Original environment file, if explicitly used")
    args = parser.parse_args()
    compose = ["compose", "--project-name", args.project_name]
    for path in args.file:
        compose += ["--file", path]
    for path in args.env_file:
        compose += ["--env-file", path]
    try:
        config = docker_json(compose + ["config", "--format", "json"], "Compose configuration inspection")
        app = docker_json(["inspect", "--type", "container", args.app_container], "Application inspection")
        mysql = docker_json(["inspect", "--type", "container", args.mysql_container], "MySQL inspection")
        require(len(app) == 1 and len(mysql) == 1, "Each explicit container selector must match one container.")
        report = check_configuration(config, app[0], mysql[0], args)
        report["database_checks"] = [inspect_database(mysql[0]["Id"], name, i == 0)
                                     for i, name in enumerate(report["databases"])]
        require(len({item["server_uuid"] for item in report["database_checks"]}) == 1,
                "Database identity changed during the preflight; rerun after resolving it.")
        report["result"] = "read-only preflight passed; backup, restore rehearsal and write draining still required"
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return 0
    except PreflightError as exc:
        print("REFUSED: " + str(exc), file=sys.stderr)
    except (KeyError, TypeError, ValueError, AttributeError):
        print("REFUSED: unsupported or incomplete Docker/Compose response; manual review required.", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
