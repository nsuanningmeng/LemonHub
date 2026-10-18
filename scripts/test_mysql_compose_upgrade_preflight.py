import contextlib
import importlib.util
import io
import json
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("preflight", Path(__file__).with_name("mysql_compose_upgrade_preflight.py"))
preflight = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(preflight)


def deployment():
    args = SimpleNamespace(app_service="new-api", mysql_service="mysql", database="new-api",
                           expected_mysql_source="production_mysql_data")
    environment = {"SQL_DSN": "user:p@ss/word@tcp(mysql:3306)/new-api?charset=utf8mb4",
                   "REDIS_CONN_STRING": "redis://:private@redis", "SESSION_SECRET": "private-session-secret"}
    config = {"name": "production", "services": {
        "new-api": {"image": "ghcr.io/example/app:v0.5.4", "environment": environment,
                    "stop_grace_period": "2m10s", "networks": {"default": {}},
                    "volumes": [{"type": "bind", "source": "/srv/app/data", "target": "/data"}]},
        "mysql": {"image": "mysql:8.2", "networks": {"default": {}},
                  "volumes": [{"type": "volume", "source": "mysql_data", "target": "/var/lib/mysql"}]}},
        "networks": {"default": {"name": "production_default"}},
        "volumes": {"mysql_data": {"name": "production_mysql_data"}}}
    app = {"Id": "app-id", "Image": "sha256:old-app", "Name": "/app",
           "Config": {"Env": [key + "=" + value for key, value in environment.items()],
                      "Labels": {"com.docker.compose.project": "production", "com.docker.compose.service": "new-api"}},
           "State": {"Status": "running", "Running": True},
           "Mounts": [{"Type": "bind", "Source": "/srv/app/data", "Destination": "/data", "RW": True}],
           "NetworkSettings": {"Networks": {"production_default": {"Aliases": ["new-api"]}}}}
    mysql = {"Id": "mysql-id", "Image": "sha256:original-mysql", "Name": "/mysql",
             "Config": {"Image": "mysql:8.2", "Env": ["MYSQL_ROOT_PASSWORD=private-root-password"],
                        "Labels": {"com.docker.compose.project": "production", "com.docker.compose.service": "mysql"}},
             "State": {"Status": "running", "Running": True},
             "Mounts": [{"Type": "volume", "Name": "production_mysql_data", "Source": "/var/lib/docker/volumes/production_mysql_data/_data",
                         "Destination": "/var/lib/mysql", "RW": True}],
             "NetworkSettings": {"Networks": {"production_default": {"Aliases": ["mysql"]}}}}
    return config, app, mysql, args


class ConfigurationTests(unittest.TestCase):
    def test_preserves_named_volume_or_original_bind_directory(self):
        for kind in ("volume", "bind"):
            with self.subTest(kind=kind):
                config, app, mysql, args = deployment()
                if kind == "bind":
                    args.expected_mysql_source = "/srv/production/mysql_data"
                    mysql["Mounts"][0].update(Type="bind", Source=args.expected_mysql_source)
                    config["services"]["mysql"]["volumes"][0].update(type="bind", source=args.expected_mysql_source)
                report = preflight.check_configuration(config, app, mysql, args)
                self.assertEqual(report["mysql_mount_source"], args.expected_mysql_source)
                self.assertEqual(report["databases"], ["new-api"])
                for secret in ("private", "p@ss/word", "SQL_DSN"):
                    self.assertNotIn(secret, json.dumps(report))

    def test_refuses_data_identity_and_unsafe_restart_changes(self):
        changes = {
            "missing DSN": lambda c, a, m, o: c["services"]["new-api"]["environment"].pop("SQL_DSN"),
            "different DSN": lambda c, a, m, o: c["services"]["new-api"]["environment"].update(SQL_DSN="user:pw@tcp(mysql:3306)/empty"),
            "wrong explicit database": lambda c, a, m, o: setattr(o, "database", "empty"),
            "wrong explicit volume": lambda c, a, m, o: setattr(o, "expected_mysql_source", "empty_volume"),
            "new volume": lambda c, a, m, o: c["volumes"]["mysql_data"].update(name="empty_volume"),
            "new bind": lambda c, a, m, o: c["services"]["mysql"]["volumes"][0].update(type="bind", source="/srv/empty"),
            "no persistent data": lambda c, a, m, o: m.update(Mounts=[]),
            "read only data": lambda c, a, m, o: m["Mounts"][0].update(RW=False),
            "nested data mount": lambda c, a, m, o: m["Mounts"].append({"Destination": "/var/lib/mysql/new-api"}),
            "project changed": lambda c, a, m, o: c.update(name="new-project"),
            "service changed": lambda c, a, m, o: m["Config"]["Labels"].update({"com.docker.compose.service": "new-mysql"}),
            "network changed": lambda c, a, m, o: c["networks"]["default"].update(name="new_default"),
            "mysql alias redirected through hosts": lambda c, a, m, o: c["services"]["new-api"].update(extra_hosts={"mysql": "203.0.113.2"}),
            "existing hosts override": lambda c, a, m, o: a.update(HostConfig={"ExtraHosts": ["mysql:203.0.113.2"]}),
            "custom DNS": lambda c, a, m, o: c["services"]["new-api"].update(dns=["203.0.113.53"]),
            "existing custom DNS search": lambda c, a, m, o: a.update(HostConfig={"DnsSearch": ["other.example"]}),
            "custom DNS search": lambda c, a, m, o: c["services"]["new-api"].update(dns_search=["other.example"]),
            "application claiming mysql alias": lambda c, a, m, o: c["services"]["new-api"]["networks"]["default"].update(aliases=["mysql"]),
            "legacy link claiming mysql alias": lambda c, a, m, o: c["services"]["new-api"].update(links=["other:mysql"]),
            "mysql image changed": lambda c, a, m, o: c["services"]["mysql"].update(image="mysql:8.4"),
            "floating app image": lambda c, a, m, o: c["services"]["new-api"].update(image="example/app:latest"),
            "candidate app image": lambda c, a, m, o: c["services"]["new-api"].update(image="example/app:v1.0.0-rc.1"),
            "implicit build": lambda c, a, m, o: c["services"]["new-api"].update(build={"context": "."}),
            "missing grace": lambda c, a, m, o: c["services"]["new-api"].pop("stop_grace_period"),
            "short grace": lambda c, a, m, o: c["services"]["new-api"].update(stop_grace_period="120s"),
            "negative internal timeout": lambda c, a, m, o: c["services"]["new-api"]["environment"].update(SHUTDOWN_TIMEOUT_SECONDS="-1"),
            "new app data": lambda c, a, m, o: c["services"]["new-api"]["volumes"][0].update(source="/srv/empty-app"),
            "changed secret": lambda c, a, m, o: c["services"]["new-api"]["environment"].update(SESSION_SECRET="new-secret"),
            "forced stop": lambda c, a, m, o: a["State"].update(Status="exited", ExitCode=137),
            "mysql stopped": lambda c, a, m, o: m["State"].update(Running=False),
        }
        for name, change in changes.items():
            with self.subTest(name=name):
                config, app, mysql, args = deployment()
                change(config, app, mysql, args)
                with self.assertRaises(preflight.PreflightError):
                    preflight.check_configuration(config, app, mysql, args)

    def test_refuses_unrecognized_dsn_hosts_and_external_log_database(self):
        for dsn in ("user:pw@tcp([::1]:3306)/new-api", "user:pw@tcp(other:3306)/new-api",
                    "user:pw@unix(/socket)/new-api", "postgresql://user:pw@mysql/new-api"):
            with self.subTest(dsn=dsn):
                config, app, mysql, args = deployment()
                config["services"]["new-api"]["environment"]["SQL_DSN"] = dsn
                app["Config"]["Env"] = ["SQL_DSN=" + dsn]
                # Keep the other preserved values equal so the DSN routing is tested.
                config["services"]["new-api"]["environment"] = {"SQL_DSN": dsn}
                with self.assertRaises(preflight.PreflightError):
                    preflight.check_configuration(config, app, mysql, args)
        config, app, mysql, args = deployment()
        config["services"]["new-api"]["environment"]["LOG_SQL_DSN"] = "user:pw@tcp(logs:3306)/logs"
        app["Config"]["Env"].append("LOG_SQL_DSN=user:pw@tcp(logs:3306)/logs")
        with self.assertRaisesRegex(preflight.PreflightError, "separate log database"):
            preflight.check_configuration(config, app, mysql, args)


class DatabaseAndCLITests(unittest.TestCase):
    valid_query = "8.2.0\t00000000-0000-4000-8000-000000000001\t/var/lib/mysql/\t80\t2\t0\t10\n"

    def test_reads_server_identity_without_injecting_database_name_into_sql(self):
        name = "db'; DROP DATABASE old; -- $(echo private)"
        with patch.object(preflight, "docker", return_value=self.valid_query) as call:
            result = preflight.inspect_database("mysql-id", name, True)
        arguments = call.call_args.args[0]
        self.assertEqual(result["database"], name)
        self.assertIn("PREFLIGHT_DATABASE=" + name, arguments)
        self.assertNotIn(name, arguments[-1])
        self.assertNotIn(name, next(arg for arg in arguments if arg.startswith("PREFLIGHT_QUERY=")))
        self.assertEqual(arguments[0], "exec")

    def test_refuses_empty_or_wrong_database_and_wrong_engine_or_datadir(self):
        for output in (self.valid_query.replace("8.2.0", "8.4.0"),
                       self.valid_query.replace("/var/lib/mysql/", "/tmp/mysql/"),
                       self.valid_query.replace("\t80\t2", "\t0\t2"),
                       self.valid_query.replace("\t80\t2", "\t80\t0"), ""):
            with self.subTest(output=output), patch.object(preflight, "docker", return_value=output):
                with self.assertRaises(preflight.PreflightError):
                    preflight.inspect_database("mysql-id", "new-api", True)

    def test_cli_emits_only_safe_report_and_only_uses_read_only_operations(self):
        config, app, mysql, _ = deployment()
        responses = [json.dumps(config), json.dumps([app]), json.dumps([mysql]), self.valid_query]
        calls = []

        def run(arguments, **kwargs):
            calls.append(arguments)
            return SimpleNamespace(returncode=0, stdout=responses.pop(0), stderr="")

        argv = ["preflight", "--file", "compose.yml", "--project-name", "production",
                "--app-container", "app", "--mysql-container", "mysql", "--database", "new-api",
                "--expected-mysql-source", "production_mysql_data"]
        out, err = io.StringIO(), io.StringIO()
        with patch.object(preflight.subprocess, "run", side_effect=run), patch.object(preflight.sys, "argv", argv), \
                contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            self.assertEqual(preflight.main(), 0)
        report = json.loads(out.getvalue())
        self.assertEqual(report["mysql_container_id"], "mysql-id")
        self.assertIn("backup", report["result"])
        self.assertEqual(err.getvalue(), "")
        self.assertNotIn("private", out.getvalue())
        self.assertEqual([args[1] for args in calls], ["compose", "inspect", "inspect", "exec"])
        self.assertIn("config", calls[0])
        self.assertIn("SELECT @@version", " ".join(calls[-1]))

    def test_cli_never_echoes_failed_docker_output(self):
        result = SimpleNamespace(returncode=1, stdout="SQL_DSN=private-password", stderr="private-root-password")
        with patch.object(preflight.subprocess, "run", return_value=result):
            with self.assertRaises(preflight.PreflightError) as error:
                preflight.docker(["inspect", "mysql"], "Inspection")
        self.assertNotIn("private", str(error.exception))


if __name__ == "__main__":
    unittest.main()
