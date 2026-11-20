"""One-use release recovery. Constants intentionally cannot be supplied by inputs."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import struct
import subprocess
import zipfile

REPO = "nsuanningmeng/LemonHub"
REPO_ID = 1267937113
TAG = "v0.5.12"
SOURCE = "86ae810bae8519aef3e20f28b65f4893f5b79ea1"
ORIGINAL_RUN = 41474646593
ORIGINAL_ATTEMPT = 2
CI_RUN = 41473646846
WORKFLOW = ".github/workflows/recover-v0.5.12.yml"
PREDICATE_TYPE = f"https://github.com/{REPO}/attestations/recovery-manifest/v1"
SLSA_TYPE = "https://slsa.dev/provenance/v1"
GROUPS = {
    "checksums-linux.txt": {"new-api-v0.5.12", "new-api-arm64-v0.5.12"},
    "checksums-windows.txt": {"new-api-v0.5.12.exe"},
    "checksums-electron-windows.txt": {
        "New-API-App.0.5.12.exe", "New-API-App.Setup.0.5.12.exe"
    },
    "checksums-macos.txt": {"new-api-macos-amd64-v0.5.12", "new-api-macos-arm64-v0.5.12"},
}
ARTIFACTS = {
    "linux-build": (13156008736, "fbb38f540c4d512dd82fab65d4d29fc6d54cc4f6d74406e569f021f5e6d26c9f", "checksums-linux.txt"),
    "windows-backend-build": (13156807836, "bed588564afc65c28893c389afc2d24a22e69e4264f1d8e7800ab3c3d6a86f02", "checksums-windows.txt"),
    "windows-electron-build": (13157781641, "81964745835698db3ca09aea2a5cba5ec6cd346fa438ec91e0aa6fff9422de1d", "checksums-electron-windows.txt"),
}
CI_JOBS = {
    "Backend vet, build, and test", "Frontend typecheck and test", "Desktop dependency audit",
    "MySQL 5.7 migration data preservation", "MySQL 8.0 migration data preservation",
    "MySQL 8.2 migration data preservation", "MySQL 8.4 migration data preservation",
    "PostgreSQL 9.6 migration data preservation", "PostgreSQL 17 migration data preservation",
    "ClickHouse existing log table migration",
}


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def api(path, method="GET", data=None):
    args = ["gh", "api", f"repos/{REPO}/{path}", "--method", method]
    if data is not None:
        args += ["--input", "-"]
    result = subprocess.run(args, input=json.dumps(data) if data is not None else None,
                            text=True, capture_output=True, check=True)
    return json.loads(result.stdout) if result.stdout else None


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def identity():
    sha = os.environ.get("RECOVERY_SHA", "")
    require(re.fullmatch(r"[0-9a-f]{40}", sha), "Missing recovery workflow SHA")
    require(os.environ.get("GITHUB_REPOSITORY") == REPO, "Wrong repository")
    require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_dispatch", "Manual dispatch required")
    require(os.environ.get("GITHUB_SHA") == sha, "Dispatch SHA differs from workflow SHA")
    require(os.environ.get("GITHUB_WORKFLOW_REF", "").startswith(f"{REPO}/{WORKFLOW}@"), "Wrong workflow")
    root = Path(__file__).resolve().parents[2]
    require(command("git", "-C", str(root), "rev-parse", "HEAD") == sha, "Verifier checkout mismatch")
    return {"repository": REPO, "workflow": WORKFLOW, "workflowCommit": sha,
            "runId": int(os.environ["GITHUB_RUN_ID"]),
            "runAttempt": int(os.environ["GITHUB_RUN_ATTEMPT"])}


def validate_jobs(jobs, expected):
    require(len(jobs) == len(expected), "Unexpected job count")
    require({job["name"] for job in jobs} == expected, "Unexpected jobs")
    require(all(job["status"] == "completed" and job["conclusion"] == "success" for job in jobs),
            "A required job did not succeed")


def guard():
    """Repeated before publishing; a queued, successful, or rerun attempt fails closed."""
    identity()
    ref = api(f"git/ref/tags/{TAG}")["object"]
    for _ in range(4):
        if ref["type"] != "tag":
            break
        ref = api(f"git/tags/{ref['sha']}")["object"]
    require(ref["type"] == "commit" and ref["sha"] == SOURCE, "Release tag moved")
    run = api(f"actions/runs/{ORIGINAL_RUN}")
    require(run["head_sha"] == SOURCE and run["head_repository"]["id"] == REPO_ID,
            "Original release source mismatch")
    require(run["path"] == ".github/workflows/release.yml" and run["event"] == "push", "Wrong original workflow")
    require(run["run_attempt"] == ORIGINAL_ATTEMPT and run["status"] == "completed"
            and run["conclusion"] in {"cancelled", "failure"}, "Original attempt 2 must have failed completely")
    jobs = api(f"actions/runs/{ORIGINAL_RUN}/attempts/{ORIGINAL_ATTEMPT}/jobs?per_page=100")["jobs"]
    mac = [j for j in jobs if j["name"] == "macOS Release"]
    require(len(mac) == 1 and mac[0]["status"] == "completed"
            and mac[0]["conclusion"] in {"failure", "cancelled"} and not mac[0].get("steps"),
            "macOS must have failed before running any build steps; inspect capacity failure manually")
    validate_jobs([j for j in jobs if j["name"] in {"Linux Release", "Windows Release"}],
                  {"Linux Release", "Windows Release"})
    require(all(j["status"] == "completed" and j["conclusion"] == "skipped"
                for j in jobs if j["name"] == "Publish complete release"), "Original publisher is not skipped")
    require(all(j["name"] in {"macOS Release", "Linux Release", "Windows Release", "Publish complete release"}
                for j in jobs), "Unexpected original release job")
    ci = api(f"actions/runs/{CI_RUN}")
    require(ci["head_sha"] == SOURCE and ci["path"] == ".github/workflows/ci.yml"
            and ci["status"] == "completed" and ci["conclusion"] == "success", "Pinned CI is not green on application source")
    validate_jobs(api(f"actions/runs/{CI_RUN}/jobs?per_page=100")["jobs"], CI_JOBS)
    return {"tagCommit": SOURCE, "originalRun": run, "originalJobs": jobs, "ciRun": ci}


def validate_artifact(meta, name, artifact_id, archive_digest):
    run = meta["workflow_run"]
    require(meta["id"] == artifact_id and meta["name"] == name and not meta["expired"], "Artifact identity/expiry mismatch")
    require(meta["digest"] == "sha256:" + archive_digest, "Artifact archive digest changed")
    require(run["id"] == ORIGINAL_RUN and run["head_sha"] == SOURCE
            and run["repository_id"] == REPO_ID and run["head_repository_id"] == REPO_ID,
            "Artifact was not built by the pinned original run/source")


def extract_archive(archive, destination, names):
    with zipfile.ZipFile(archive) as zip_file:
        infos = zip_file.infolist()
        require(len(infos) == len(names) and {info.filename for info in infos} == names, "Unexpected ZIP members")
        for info in infos:
            require(info.filename == Path(info.filename).name and not info.is_dir(), "Unsafe ZIP path")
            mode = info.external_attr >> 16
            require(not stat.S_ISLNK(mode) and 0 < info.file_size < 1024**3, "Unsafe ZIP member")
            target = destination / info.filename
            require(not target.exists(), "Refusing to overwrite staged asset")
            with zip_file.open(info) as source, target.open("xb") as out:
                shutil.copyfileobj(source, out)


def validate_checksums(directory, checksum_name):
    entries = {}
    for line in (directory / checksum_name).read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64}) [ *]([^/\\]+)", line)
        require(match is not None, "Invalid checksum entry")
        expected, name = match.groups()
        require(name not in entries, "Duplicate checksum entry")
        entries[name] = expected
    require(set(entries) == GROUPS[checksum_name], "Checksum member set mismatch")
    for name, expected in entries.items():
        require((directory / name).is_file() and not (directory / name).is_symlink(), "Invalid release file")
        require(digest(directory / name) == expected, f"Checksum mismatch: {name}")


def verify_attestation(path, workflow, source, bundle=None, predicate_type=SLSA_TYPE):
    args = ["gh", "attestation", "verify", str(path), "--repo", REPO,
            "--signer-workflow", f"{REPO}/{workflow}", "--source-digest", source,
            "--deny-self-hosted-runners", "--predicate-type", predicate_type]
    if bundle is not None:
        args += ["--bundle", str(bundle)]
    subprocess.run(args, check=True)


def release_notes(application):
    recovery = identity()
    root = Path(__file__).resolve().parents[2]
    verification = (root / ".github/release-notes/v0.5.12-verification.md").read_text()
    require(verification.strip(), "Verified release addendum is empty")
    notes = command("git", "-C", str(application), "show", f"{SOURCE}:.github/release-notes/{TAG}.md")
    run_url = f"https://github.com/{REPO}/actions/runs/{recovery['runId']}/attempts/{recovery['runAttempt']}"
    commit_url = f"https://github.com/{REPO}/commit/{recovery['workflowCommit']}"
    recovery_notes = (
        "## macOS 产物恢复说明\n\n"
        "原发布任务因 GitHub 托管 macOS runner 容量不足，在执行任何 macOS 构建步骤前失败。"
        "本次恢复使用 Ubuntu 托管 runner、Go 1.26.10 和 Bun 1.4.4，从同一应用源码完整构建前端，"
        "并在禁用 CGO 的条件下交叉编译 Darwin amd64/arm64；未改变应用标签或源码提交。\n\n"
        f"Linux、Windows 和 Electron 产物保持[原发布任务](https://github.com/{REPO}/actions/runs/{ORIGINAL_RUN})的原始字节，"
        "已重新核对校验和及原构建证明。"
        f"[本次恢复工作流]({run_url})使用独立的[工作流提交]({commit_url})；"
        "标准构建证明记录该恢复身份，附加签名清单同时绑定原应用源码、恢复工作流、全部产物摘要及本页完整说明。"
    )
    return "\n\n".join((notes.strip(), verification.strip(), recovery_notes)) + "\n"


def prepare(payload, application):
    # No success is inferred from cached files: a fresh payload directory is required.
    require(not payload.exists(), "Use an empty output directory")
    require(command("git", "-C", str(application), "rev-parse", "HEAD") == SOURCE, "Application checkout mismatch")
    evidence = guard()
    subprocess.run(["gh", "attestation", "verify", "--help"], check=True, stdout=subprocess.DEVNULL)
    stage = payload / "release"
    proof = payload / "proof"
    stage.mkdir(parents=True)
    proof.mkdir()
    write_json(proof / "initial-gates.json", evidence)
    for name, (artifact_id, archive_digest, checksums) in ARTIFACTS.items():
        meta = api(f"actions/artifacts/{artifact_id}")
        validate_artifact(meta, name, artifact_id, archive_digest)
        write_json(proof / f"{name}.json", meta)
        archive = proof / f"{name}.zip"
        with archive.open("xb") as out:
            subprocess.run(["gh", "api", f"repos/{REPO}/actions/artifacts/{artifact_id}/zip"], stdout=out, check=True)
        require(digest(archive) == archive_digest, "Downloaded ZIP digest mismatch")
        extract_archive(archive, stage, GROUPS[checksums] | {checksums})
        archive.unlink()
        validate_checksums(stage, checksums)
        for binary in sorted(GROUPS[checksums]):
            verify_attestation(stage / binary, ".github/workflows/release.yml", SOURCE)
    (proof / "release-notes.md").write_text(release_notes(application))


def release_digests(stage):
    names = set(GROUPS).union(*GROUPS.values())
    require(stage.is_dir() and {p.name for p in stage.iterdir()} == names, "Expected exactly 11 release assets")
    for name in names:
        require((stage / name).is_file() and not (stage / name).is_symlink(), "Nonregular release asset")
    for checksums in GROUPS:
        validate_checksums(stage, checksums)
    return {name: digest(stage / name) for name in sorted(names)}


def manifest(payload, application):
    recovery = identity()
    require(command("git", "-C", str(application), "rev-parse", "HEAD") == SOURCE, "Application checkout mismatch")
    subprocess.run(["git", "-C", str(application), "diff", "--exit-code"], check=True)
    stage, proof = payload / "release", payload / "proof"
    hashes = release_digests(stage)
    build_info = {}
    for architecture, cpu in {"amd64": 0x01000007, "arm64": 0x0100000C}.items():
        binary = stage / f"new-api-macos-{architecture}-{TAG}"
        with binary.open("rb") as file:
            magic, actual_cpu = struct.unpack("<II", file.read(8))
        require(magic == 0xFEEDFACF and actual_cpu == cpu, "Wrong Mach-O architecture")
        info = command("go", "version", "-m", str(binary))
        require("go1.26.10" in info and "GOOS=darwin" in info and f"GOARCH={architecture}" in info
                and "CGO_ENABLED=0" in info and f"vcs.revision={SOURCE}" in info
                and "vcs.modified=false" in info and f"common.Version={TAG}" in info,
                "Incorrect macOS build metadata")
        build_info[architecture] = info
    write_json(proof / "macos-build-info.json", build_info)
    origins = {}
    for name, (artifact_id, archive_digest, checksum) in ARTIFACTS.items():
        for asset in GROUPS[checksum] | {checksum}:
            origins[asset] = {"runId": ORIGINAL_RUN, "artifactId": artifact_id,
                              "artifactName": name, "archiveSha256": archive_digest}
    for asset in GROUPS["checksums-macos.txt"] | {"checksums-macos.txt"}:
        origins[asset] = {"runId": recovery["runId"], "workflowCommit": recovery["workflowCommit"]}
    value = {
        "schemaVersion": 1,
        "application": {"repository": REPO, "tag": TAG, "gitCommit": SOURCE},
        "recovery": recovery,
        "releaseNotesSha256": digest(proof / "release-notes.md"),
        "validation": {"ciRunId": CI_RUN, "originalReleaseRunId": ORIGINAL_RUN, "originalAttempt": ORIGINAL_ATTEMPT},
        "build": {"host": "ubuntu-latest", "go": "1.26.10", "bun": "1.4.4", "cgo": False,
                  "targets": ["darwin/amd64", "darwin/arm64"], "originalBinaryBytesPreserved": True},
        "artifacts": [{"name": name, "sha256": hashes[name], "origin": origins[name]} for name in hashes],
    }
    write_json(proof / "recovery-manifest.json", value)


def verify(payload):
    recovery = identity()
    stage, proof = payload / "release", payload / "proof"
    hashes = release_digests(stage)
    value = json.loads((proof / "recovery-manifest.json").read_text())
    require(value["application"] == {"repository": REPO, "tag": TAG, "gitCommit": SOURCE}, "Manifest application mismatch")
    require(value["recovery"] == recovery, "Manifest recovery identity mismatch")
    require(value["releaseNotesSha256"] == digest(proof / "release-notes.md"), "Release notes do not match signed manifest")
    require(len(value["artifacts"]) == len(hashes)
            and {a["name"]: a["sha256"] for a in value["artifacts"]} == hashes, "Manifest asset digest mismatch")
    bundle = proof / "recovery-manifest.sigstore.json"
    statement = json.loads(base64.b64decode(json.loads(bundle.read_text())["dsseEnvelope"]["payload"], validate=True))
    require(statement["predicateType"] == PREDICATE_TYPE and statement["predicate"] == value, "Bundle is not the staged manifest")
    require(len(statement["subject"]) == len(hashes)
            and {Path(s["name"]).name: s["digest"]["sha256"] for s in statement["subject"]} == hashes,
            "Signed subject list mismatch")
    for name in hashes:
        verify_attestation(stage / name, WORKFLOW, recovery["workflowCommit"], bundle, PREDICATE_TYPE)
    for binary in GROUPS["checksums-macos.txt"]:
        verify_attestation(stage / binary, WORKFLOW, recovery["workflowCommit"], proof / "macos-provenance.sigstore.json")
    # Preserve original provenance as a separate requirement, not just a claim in the new manifest.
    for _, _, checksums in ARTIFACTS.values():
        for binary in GROUPS[checksums]:
            verify_attestation(stage / binary, ".github/workflows/release.yml", SOURCE)
    return hashes


def validate_release_assets(assets, hashes, allow_missing=False):
    require(len(assets) == len({a["name"] for a in assets}), "Duplicate remote release asset")
    require({a["name"] for a in assets} <= hashes.keys(), "Unexpected existing release assets; refusing mutation")
    if not allow_missing:
        require({a["name"] for a in assets} == hashes.keys(), "Incomplete remote release")
    for asset in assets:
        require(asset["state"] == "uploaded" and asset["size"] > 0
                and asset.get("digest") == "sha256:" + hashes[asset["name"]], "Existing asset differs; refusing overwrite")


def publish(payload):
    hashes = verify(payload)
    guard()
    # List with pagination rather than treating an authentication/API error as 'no release'.
    releases = json.loads(command("gh", "api", "--paginate", "--slurp", f"repos/{REPO}/releases?per_page=100"))
    matches = [release for page in releases for release in page if release["tag_name"] == TAG]
    require(len(matches) <= 1, "Duplicate tag releases")
    if matches:
        release = matches[0]
        require(release["draft"] and not release["prerelease"], "Release is already public or wrong release kind; stop")
        require(release.get("body") == (payload / "proof/release-notes.md").read_text(),
                "Existing draft has different notes; inspect it instead of overwriting")
    else:
        release = api("releases", "POST", {"tag_name": TAG, "target_commitish": SOURCE, "name": TAG,
                      "draft": True, "prerelease": False, "body": (payload / "proof/release-notes.md").read_text()})
    release_id = release["id"]
    require(release["tag_name"] == TAG and release["draft"], "Draft creation mismatch")
    assets = api(f"releases/{release_id}/assets?per_page=100")
    validate_release_assets(assets, hashes, allow_missing=True)
    present = {a["name"] for a in assets}
    for name in hashes:
        if name not in present:
            subprocess.run(["gh", "release", "upload", TAG, str(payload / "release" / name), "--repo", REPO], check=True)
    validate_release_assets(api(f"releases/{release_id}/assets?per_page=100"), hashes)
    guard()
    current = api(f"releases/{release_id}")
    require(current["tag_name"] == TAG and current["draft"] and not current["prerelease"], "Draft changed during upload")
    require(current.get("body") == (payload / "proof/release-notes.md").read_text(),
            "Draft notes changed after signature verification")
    validate_release_assets(api(f"releases/{release_id}/assets?per_page=100"), hashes)
    api(f"releases/{release_id}", "PATCH", {"draft": False, "make_latest": "legacy"})
    current = api(f"releases/{release_id}")
    require(not current["draft"] and not current["prerelease"] and current["tag_name"] == TAG, "Publication did not complete")
    validate_release_assets(api(f"releases/{release_id}/assets?per_page=100"), hashes)
    print(f"Published complete verified release: {current['html_url']}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=["prepare", "manifest", "verify", "publish"])
    parser.add_argument("--payload", required=True, type=Path)
    parser.add_argument("--application", type=Path)
    args = parser.parse_args()
    if args.operation in {"prepare", "manifest"}:
        require(args.application is not None, "Application checkout is required")
        globals()[args.operation](args.payload, args.application)
    else:
        globals()[args.operation](args.payload)
