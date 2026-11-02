"""Local tests of failure-closed recovery boundaries; no network or GitHub writes."""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

SPEC = importlib.util.spec_from_file_location("recovery", Path(__file__).with_name("recover_v0512.py"))
recovery = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(recovery)


class RecoveryBoundaryTests(unittest.TestCase):
    def test_release_notes_include_verified_addendum_and_actual_recovery_identity(self):
        workflow_commit = "a" * 40
        context = {"runId": 123456, "runAttempt": 2, "workflowCommit": workflow_commit}
        application = Path("/tagged-application")
        with patch.object(recovery, "identity", return_value=context), \
             patch.object(recovery, "command", return_value="tagged source notes") as command:
            notes = recovery.release_notes(application)
        command.assert_called_once_with("git", "-C", str(application), "show",
                                        f"{recovery.SOURCE}:.github/release-notes/v0.5.12.md")
        addendum = (Path(__file__).resolve().parents[2] / ".github/release-notes/v0.5.12-verification.md").read_text().strip()
        self.assertTrue(notes.startswith("tagged source notes\n\n" + addendum + "\n\n"))
        self.assertEqual(notes.count("## 正式产物验收（2026-11-02）"), 1)
        self.assertIn("## macOS 产物恢复说明", notes)
        self.assertIn("https://github.com/nsuanningmeng/LemonHub/actions/runs/123456/attempts/2", notes)
        self.assertIn("https://github.com/nsuanningmeng/LemonHub/commit/" + workflow_commit, notes)
        self.assertIn("全部产物摘要及本页完整说明", notes)

    def test_checksum_valid_and_tampered_binary(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            name = "new-api-v0.5.12.exe"
            (directory / name).write_bytes(b"original release binary")
            checksum = "checksums-windows.txt"
            (directory / checksum).write_text(f"{recovery.digest(directory / name)}  {name}\n")
            recovery.validate_checksums(directory, checksum)
            (directory / name).write_bytes(b"changed")
            with self.assertRaisesRegex(RuntimeError, "Checksum mismatch"):
                recovery.validate_checksums(directory, checksum)

    def test_duplicate_checksum_entry_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            line = f"{'0' * 64}  new-api-v0.5.12.exe\n"
            (directory / "checksums-windows.txt").write_text(line + line)
            with self.assertRaisesRegex(RuntimeError, "Duplicate checksum"):
                recovery.validate_checksums(directory, "checksums-windows.txt")

    def test_zip_rejects_traversal_and_duplicate_member(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            archive = directory / "input.zip"
            for names in [["../escape"], ["binary", "binary"]]:
                with zipfile.ZipFile(archive, "w") as output:
                    for name in names:
                        output.writestr(name, b"data")
                with self.assertRaisesRegex(RuntimeError, "Unexpected ZIP"):
                    recovery.extract_archive(archive, directory, {"binary"})
            self.assertFalse((directory.parent / "escape").exists())

    def test_artifact_is_bound_to_original_commit_and_run(self):
        name, (artifact_id, digest, _) = next(iter(recovery.ARTIFACTS.items()))
        metadata = {"id": artifact_id, "name": name, "expired": False, "digest": "sha256:" + digest,
                    "workflow_run": {"id": recovery.ORIGINAL_RUN, "head_sha": recovery.SOURCE,
                                     "repository_id": recovery.REPO_ID, "head_repository_id": recovery.REPO_ID}}
        recovery.validate_artifact(metadata, name, artifact_id, digest)
        metadata["workflow_run"]["head_sha"] = "0" * 40
        with self.assertRaisesRegex(RuntimeError, "original run/source"):
            recovery.validate_artifact(metadata, name, artifact_id, digest)

    def test_remote_asset_mismatch_and_unexpected_asset_rejected(self):
        expected = {"binary": hashlib.sha256(b"content").hexdigest()}
        asset = {"name": "binary", "state": "uploaded", "size": 7, "digest": "sha256:" + expected["binary"]}
        recovery.validate_release_assets([asset], expected)
        recovery.validate_release_assets([], expected, allow_missing=True)
        for assets in [[], [dict(asset, digest="sha256:" + "0" * 64)], [dict(asset, name="other")], [asset, asset]]:
            with self.assertRaises(RuntimeError):
                recovery.validate_release_assets(assets, expected)

    def test_ci_skipped_job_is_not_success(self):
        with self.assertRaisesRegex(RuntimeError, "did not succeed"):
            recovery.validate_jobs([{"name": "required", "status": "completed", "conclusion": "skipped"}], {"required"})

    def test_original_queued_attempt_cannot_enter_recovery(self):
        run = {"head_sha": recovery.SOURCE, "head_repository": {"id": recovery.REPO_ID},
               "path": ".github/workflows/release.yml", "event": "push", "run_attempt": 2,
               "status": "in_progress", "conclusion": None}
        def fake_api(path):
            if path.startswith("git/ref/"):
                return {"object": {"type": "commit", "sha": recovery.SOURCE}}
            return run
        with patch.object(recovery, "identity"), patch.object(recovery, "api", side_effect=fake_api):
            with self.assertRaisesRegex(RuntimeError, "attempt 2 must have failed"):
                recovery.guard()

    def test_moved_tag_stops_before_release_api(self):
        with patch.object(recovery, "identity"), patch.object(recovery, "api", return_value={
            "object": {"type": "commit", "sha": "0" * 40}
        }) as api:
            with self.assertRaisesRegex(RuntimeError, "tag moved"):
                recovery.guard()
            self.assertEqual(api.call_count, 1)

    def test_published_release_is_never_overwritten(self):
        with tempfile.TemporaryDirectory() as temporary:
            payload = Path(temporary)
            with patch.object(recovery, "verify", return_value={"binary": "0" * 64}), \
                 patch.object(recovery, "guard"), \
                 patch.object(recovery, "command", return_value='[[{"tag_name":"v0.5.12","draft":false,"prerelease":false}]]'), \
                 patch.object(recovery, "api") as api:
                with self.assertRaisesRegex(RuntimeError, "already public"):
                    recovery.publish(payload)
                api.assert_not_called()

    def test_concurrent_note_change_keeps_release_as_draft(self):
        with tempfile.TemporaryDirectory() as temporary:
            payload = Path(temporary)
            (payload / "proof").mkdir()
            (payload / "proof/release-notes.md").write_text("signed notes")
            release = {"id": 1, "tag_name": recovery.TAG, "draft": True, "prerelease": False, "body": "signed notes"}
            asset = {"name": "binary", "state": "uploaded", "size": 1, "digest": "sha256:" + "0" * 64}
            def fake_api(path, method="GET", data=None):
                self.assertEqual(method, "GET", "A changed draft must never be published")
                if "/assets?" in path:
                    return [asset]
                return dict(release, body="notes changed concurrently")
            with patch.object(recovery, "verify", return_value={"binary": "0" * 64}), \
                 patch.object(recovery, "guard"), \
                 patch.object(recovery, "command", return_value=json.dumps([[release]])), \
                 patch.object(recovery, "api", side_effect=fake_api):
                with self.assertRaisesRegex(RuntimeError, "Draft notes changed"):
                    recovery.publish(payload)


if __name__ == "__main__":
    unittest.main()
