import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

from PIL import Image


class SmokeContract(unittest.TestCase):
    def setUp(self):
        path = Path(__file__).with_name("verify.py")
        self.assertTrue(path.exists(), "real smoke runner is missing")
        spec = importlib.util.spec_from_file_location("verify", path)
        self.runner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.runner)

    def test_contract_covers_all_five_critical_journeys(self):
        self.assertEqual(set(self.runner.JOURNEYS), {
            "TestE2ECoreBrowserJourney", "TestE2ESearchEngineSettingsJourney",
            "TestE2EFilePickerSelectionReachesScript", "TestE2EFindJourney",
            "TestE2ETabIsolationJourney",
        })

    def test_requires_every_journey_to_run(self):
        records = [json.dumps({"Action": "pass", "Test": name}) for name in self.runner.JOURNEYS]
        self.runner.check_journeys("\n".join(records))
        with self.assertRaises(ValueError):
            self.runner.check_journeys("\n".join(records[:-1]))
        with self.assertRaises(ValueError):
            self.runner.check_journeys("\n".join(records + [json.dumps({"Action": "skip", "Test": "anything"})]))

    def test_timeout_retains_output_and_returns_failure(self):
        import sys
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "timeout.log"
            result = self.runner.execute([sys.executable, "-u", "-c", "import threading; print('started', flush=True); threading.Event().wait()"], Path(directory), log, 1)
            self.assertEqual(result, 124)
            self.assertIn("started", log.read_text())

    def test_rejects_blank_or_wrong_pixels(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "shot.png"
            image = Image.new("RGB", (320, 200), "white")
            image.save(path)
            with self.assertRaises(ValueError):
                self.runner.check_image(path)
            image.paste((255, 0, 0), (16, 16, 112, 80))
            image.paste((0, 128, 0), (160, 16, 256, 80))
            image.save(path)
            self.runner.check_image(path)
            image.resize((160, 100)).save(path)
            with self.assertRaises(ValueError):
                self.runner.check_image(path)


if __name__ == "__main__":
    unittest.main()
