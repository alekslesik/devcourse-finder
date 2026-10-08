"""Exercise an old Compose deployment command after a worker has been added."""
import json
import os
import pathlib
import subprocess
import tempfile
import unittest
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[1]


class DeploymentPolicyTests(unittest.TestCase):
    def test_current_script_and_legacy_ssh_command_remove_orphans(self):
        script = (ROOT / "scripts/deploy-release.sh").read_text()
        workflow = (ROOT / ".github/workflows/deploy-release.yml").read_text()
        self.assertIn("up --build -d --remove-orphans --wait", script)
        # The selected tag supplies the deployment script. Fixing only that
        # script would not fix old immutable tags: the SSH launcher must set it.
        self.assertIn('"$target" "env COMPOSE_REMOVE_ORPHANS=1 bash -s --', workflow)

    @unittest.skipUnless(os.environ.get("RUN_DOCKER_DEPLOY_TESTS") == "1", "requires isolated Docker fixtures")
    def test_legacy_command_removes_worker_and_preserves_application_volume(self):
        project = "devcourse-rollback-test-" + uuid.uuid4().hex[:12]
        image = os.environ.get("DEPLOY_TEST_IMAGE", "postgres:17-alpine")
        # No production configuration, daemon restart, or production volume is
        # used. Compose labels scope orphan removal to this unique test project.
        app = {"image": image, "entrypoint": ["sleep", "600"], "volumes": ["persistent:/data"]}
        older = {"services": {"app": app}, "volumes": {"persistent": {}}}
        newer = {"services": {"app": app, "catalog-updater": {
            "image": image, "entrypoint": ["sleep", "600"], "restart": "unless-stopped",
        }}, "volumes": {"persistent": {}}}
        settings = os.environ.copy()
        settings["COMPOSE_REMOVE_ORPHANS"] = "1"
        settings.pop("COMPOSE_IGNORE_ORPHANS", None)
        with tempfile.TemporaryDirectory() as directory:
            old_file = pathlib.Path(directory) / "old.json"
            new_file = pathlib.Path(directory) / "new.json"
            old_file.write_text(json.dumps(older))
            new_file.write_text(json.dumps(newer))
            prefix = ["docker", "compose", "--project-name", project]

            def run(args):
                return subprocess.run(args, env=settings, check=True, capture_output=True, text=True, timeout=90).stdout.strip()

            try:
                run(prefix + ["-f", str(new_file), "up", "-d"])
                worker = run(prefix + ["-f", str(new_file), "ps", "-q", "catalog-updater"])
                self.assertTrue(worker)
                run(prefix + ["-f", str(new_file), "exec", "-T", "app", "sh", "-c", "echo retained > /data/marker"])
                # This deliberately uses the old script's up command, without
                # --remove-orphans. The SSH launcher's environment fixes it.
                run(prefix + ["-f", str(old_file), "up", "--build", "-d", "--wait", "--wait-timeout", "30"])
                remaining = run(["docker", "ps", "-aq", "--filter", "label=com.docker.compose.project=" + project])
                self.assertNotIn(worker, remaining.splitlines())
                self.assertEqual(len(remaining.splitlines()), 1)
                self.assertEqual(run(prefix + ["-f", str(old_file), "exec", "-T", "app", "cat", "/data/marker"]), "retained")
            finally:
                run(prefix + ["-f", str(new_file), "down", "--volumes", "--remove-orphans"])


if __name__ == "__main__":
    unittest.main()
