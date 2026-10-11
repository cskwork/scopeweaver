import copy
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile

spec = importlib.util.spec_from_file_location("repair", Path(__file__).with_name("repair-release-notices.py"))
repair = importlib.util.module_from_spec(spec)
spec.loader.exec_module(repair)
NOTICE = Path(__file__).resolve().parents[1].joinpath(repair.NOTICE).read_bytes()


class ReleaseNoticeTests(unittest.TestCase):
    def fixture(self, root):
        source = root / "original"
        source.mkdir()
        sums = []
        for target in repair.TARGETS:
            name = f"scopeweaver-0.2.1-{target}"
            with zipfile.ZipFile(source / f"{name}.zip", "w") as archive:
                binary = "scopeweaver.exe" if target.startswith("windows") else "scopeweaver"
                info = zipfile.ZipInfo(f"{name}/{binary}")
                info.external_attr = 0o100755 << 16
                archive.writestr(info, b"original executable\x00\xff" + target.encode())
                archive.writestr(f"{name}/docs/guide.md", b"original documentation")
            sums.append(f"{repair.sha256_file(source / (name + '.zip'))}  {name}.zip\n")
        (source / "SHA256SUMS").write_text("".join(sums))
        return source

    def test_all_archives_preserve_original_entries_permissions_and_binaries(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = self.fixture(root)
            output = root / "repaired"
            repair.repair_archives(source, output, NOTICE)
            self.assertEqual((source / "SHA256SUMS").read_bytes(), (output / "SHA256SUMS.original").read_bytes())
            for original in source.glob("*.zip"):
                with zipfile.ZipFile(original) as before, zipfile.ZipFile(output / original.name) as after:
                    self.assertEqual(len(after.infolist()), len(before.infolist()) + 1)
                    for member in before.infolist():
                        self.assertEqual(before.read(member), after.read(member.filename))
                        self.assertEqual(member.external_attr, after.getinfo(member.filename).external_attr)
                    self.assertEqual(after.read(f"{original.stem}/{repair.NOTICE}"), NOTICE)

    def test_wrong_original_checksum_fails_before_output_is_created(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = self.fixture(root)
            archive = next(source.glob("*.zip"))
            archive.write_bytes(archive.read_bytes() + b"changed")
            with self.assertRaisesRegex(ValueError, "Original checksum mismatch"):
                repair.repair_archives(source, root / "repaired", NOTICE)
            self.assertFalse((root / "repaired").exists())

    def test_changed_original_file_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = self.fixture(root)
            real_entries = repair.archive_entries

            def corrupt_replacement_hash(archive):
                entries = real_entries(archive)
                if Path(archive).parent.name == "repaired":
                    entries[next(name for name in entries if "scopeweaver" == name.split("/")[-1])] = "0" * 64
                return entries

            with patch.object(repair, "archive_entries", side_effect=corrupt_replacement_hash):
                with self.assertRaisesRegex(ValueError, "Original file changed"):
                    repair.repair_archives(source, root / "repaired", NOTICE)

    def test_image_preserves_layers_and_runtime_configuration(self):
        before = {"layers": [{"digest": "original-layer"}]}
        after = {"layers": [{"digest": "original-layer"}, {"digest": "notice-layer"}]}
        config = {"os": "linux", "architecture": "amd64", "config": {"Cmd": ["unchanged"]}, "rootfs": {"diff_ids": ["original-diff"]}}
        fixed = copy.deepcopy(config)
        fixed["rootfs"]["diff_ids"].append("notice-diff")
        repair.verify_image_config(before, after, config, fixed)
        for changed in ("layer", "configuration", "uncompressed-layer"):
            with self.subTest(changed=changed):
                bad_manifest, bad_config = copy.deepcopy(after), copy.deepcopy(fixed)
                if changed == "layer":
                    bad_manifest["layers"][0]["digest"] = "changed"
                elif changed == "configuration":
                    bad_config["config"]["Cmd"] = ["different"]
                else:
                    bad_config["rootfs"]["diff_ids"][0] = "changed"
                with self.assertRaises(ValueError):
                    repair.verify_image_config(before, bad_manifest, config, bad_config)

    def test_notice_layer_rejects_modified_notice_or_application_file(self):
        for name, body, valid in ((f"app/{repair.NOTICE}", NOTICE, True),
                                  (f"app/{repair.NOTICE}", b"modified notice", False),
                                  ("app/scopeweaver", b"changed application", False)):
            with self.subTest(name=name, valid=valid):
                data = io.BytesIO()
                with tarfile.open(fileobj=data, mode="w:gz") as archive:
                    directory = tarfile.TarInfo("app/")
                    directory.type = tarfile.DIRTYPE
                    archive.addfile(directory)
                    member = tarfile.TarInfo(name)
                    member.size = len(body)
                    archive.addfile(member, io.BytesIO(body))
                if valid:
                    repair.verify_notice_layer(data.getvalue(), NOTICE)
                else:
                    with self.assertRaises(ValueError):
                        repair.verify_notice_layer(data.getvalue(), NOTICE)


if __name__ == "__main__":
    unittest.main()
