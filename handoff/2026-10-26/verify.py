"""Verify the portable evidence package; optionally extract into a new directory."""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import tarfile

base = Path(__file__).resolve().parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--extract', type=Path, help='new, nonexistent destination directory')
args = parser.parse_args()
manifest = json.loads((base / 'MANIFEST.json').read_text())
result = json.loads((base / 'PACKAGE-RESULT.json').read_text())
archive = base / 'EVIDENCE.tar.gz'
if hashlib.sha256(archive.read_bytes()).hexdigest() != result['archive_sha256']:
    raise SystemExit('Archive checksum mismatch')
expected = {row['path']: row for row in manifest['files'] if row['status'] == 'included'}
if args.extract and args.extract.exists():
    raise SystemExit('Extraction destination already exists; choose a new directory')
with tarfile.open(archive, 'r:gz') as package:
    members = package.getmembers()
    if len(members) != len(expected) or {m.name for m in members} != set(expected):
        raise SystemExit('Archive member inventory mismatch')
    for member in members:
        path = PurePosixPath(member.name)
        if not member.isfile() or path.is_absolute() or '..' in path.parts:
            raise SystemExit('Unsafe archive member: ' + member.name)
        data = package.extractfile(member).read()
        row = expected[member.name]
        if len(data) != row['published_bytes'] or hashlib.sha256(data).hexdigest() != row['published_sha256']:
            raise SystemExit('Member checksum mismatch: ' + member.name)
    if args.extract:
        args.extract.mkdir(parents=True, exist_ok=False)
        for member in members:
            dest = args.extract / member.name
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(package.extractfile(member).read())
print(f"PASS: {len(members)} evidence files; archive and every published file checksum verified")
print('Source hashes identify local originals; redacted/normalized copies are not byte-identical original evidence.')
