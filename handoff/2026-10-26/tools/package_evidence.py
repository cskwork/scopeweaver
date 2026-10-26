from pathlib import Path
from collections import Counter
import gzip
import hashlib
import io
import json
import re
import tarfile

BASE = Path('/Users/danny/Documents/Git/scopeweaver-worktrees')
DEST = BASE / 'handoff-evidence-20261026/handoff/2026-10-26'
STAGE = BASE / 'handoff-publication-20261026/sanitized'
ROOTS = {
    'pr40': BASE / 'pr40-review-repair/.sdlc/work/261025-pr40-review-repair',
    'pr46': BASE / 'pr46-ci-cancellation/.sdlc/archive/261025-pr46-ci-cancellation',
    'gh49-plan': BASE / 'gh49-opencode-sdk/.sdlc/work/gh-49-opencode-support',
    'gh49-evidence': BASE / 'gh49-recovery-evidence',
    'gh15': BASE / 'gh49-opencode-sdk/.sdlc/work/gh-15-complete-locales',
    'gh50': Path('/Users/danny/Documents/Git/scopeweaver/.sdlc/archive/gh-50-scope-enforcement'),
    'gh53-verification': BASE / 'gh53-verification-gates/.sdlc/work/gh-53-verification-gates',
    'gh53-retry': BASE / 'gh53-verification-gates/.sdlc/work/gh-53-retry-dialog',
    'release': BASE / 'gh53-verification-gates/.sdlc/work/integrated-release-v0-8',
}
SKIP_DIRS = {'.git', 'node_modules', '.cache', '__pycache__', 'pgdata', 'pg_wal', 'pg', '.venv', 'venv'}
VISUALS = {'.png', '.jpg', '.jpeg', '.gif', '.webp', '.svg', '.mp4', '.webm'}
CONTAINERS = {'.zip', '.gz', '.tar', '.bundle', '.pile'}
SECRET_NAMES = {'auth.json', 'credentials.json', 'cookies.json', 'storage-state.json', 'storageState.json', 'id_rsa', 'id_ed25519', '.npmrc', '.pgpass'}
DEST.mkdir(parents=True, exist_ok=True)
STAGE.mkdir(parents=True, exist_ok=True)
masked = {}


def mask(value):
    if not value or '__REDACTED_' in value or value.startswith(('$', '${', '<')):
        return value
    if value.isdecimal():
        return value
    if value not in masked:
        masked[value] = f'__REDACTED_{len(masked)+1:06d}__'
    return masked[value]


KEYS = r'(?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|authorization|cookie|set-cookie|password|passwd|secret|token)'
NORMAL = re.compile(r'(?i)(["\']'+KEYS+r'["\']\s*:\s*)(["\'])((?:\\.|[^\\\r\n])*?)\2')
ESCAPED = re.compile(r'(?i)(\\"'+KEYS+r'\\"\s*:\s*\\")((?:\\\\.|[^"\\\r\n])*?)(\\")')
UNQUOTED = re.compile(r'(?i)(\b'+KEYS+r'\s*:\s*)(["\'])((?:\\.|[^\\\r\n])*?)\2')
BEARER = re.compile(r'(?i)\b(Bearer|Basic)(\s+)([A-Za-z0-9._~+/=-]{8,})')
TOKEN = re.compile(r'\b(?:github_pat_[A-Za-z0-9_]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|sk-(?:proj-)?[A-Za-z0-9_-]{20,}|AIza[0-9A-Za-z_-]{30,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})')
ENV = re.compile(r'(?m)(\b(?:[A-Z][A-Z0-9_]*_)?(?:API_KEY|TOKEN|PASSWORD|SECRET|CREDENTIAL)[A-Z0-9_]*\s*=\s*)("[^"\r\n]*"|\'[^\'\r\n]*\'|[^\s;]+)')
URI = re.compile(r'(?i)((?:postgres(?:ql)?|https?|redis|mysql|mongodb(?:\+srv)?)://[^\s/@:]+:)([^\s/@]+)(@)')
SIGNED_QUERY = re.compile(r'(?i)([?&](?:sig|signature|token|access_token|X-Amz-Signature|X-Amz-Credential|X-Amz-Security-Token)=)([^&\s"\']+)')
PEM = re.compile(r'-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----', re.S)


def sanitize(text):
    text = PEM.sub(lambda m: mask(m.group()), text)
    text = TOKEN.sub(lambda m: mask(m.group()), text)
    text = BEARER.sub(lambda m: m[1]+m[2]+mask(m[3]), text)
    for pattern in (NORMAL, UNQUOTED):
        text = pattern.sub(lambda m: m[1]+m[2]+mask(m[3])+m[2], text)
    text = ESCAPED.sub(lambda m: m[1]+mask(m[2])+m[3], text)
    def env_replace(m):
        val=m[2]
        if val[:1] in ('"', "'"):
            return m[1]+val[0]+mask(val[1:-1])+val[-1]
        return m[1]+mask(val)
    text = ENV.sub(env_replace, text)
    text = URI.sub(lambda m: m[1]+mask(m[2])+m[3], text)
    text = SIGNED_QUERY.sub(lambda m: m[1]+mask(m[2]), text)
    return text


records = []
for label, root in ROOTS.items():
    assert root.is_dir(), root
    for src in sorted(root.rglob('*')):
        if src.is_dir():
            continue
        rel = src.relative_to(root)
        rec = {'path': label+'/'+str(rel), 'source_bytes': src.lstat().st_size}
        reason = None
        if src.is_symlink():
            reason = 'symlink: target not copied'
        elif any(part in SKIP_DIRS for part in rel.parts):
            reason = 'database/cache/dependency/runtime directory'
        elif src.name in SECRET_NAMES or (src.name.startswith('.env') and not src.name.endswith(('.example', '.sample'))):
            reason = 'credential/session/config file'
        elif src.suffix.lower() in VISUALS:
            reason = 'visual/binary artifact; not included without content-specific privacy review'
        elif src.suffix.lower() in CONTAINERS:
            reason = 'container/bundle; extracted textual evidence copied separately where available'
        elif src.suffix.lower() in {'.test', '.db', '.sqlite', '.sqlite3', '.so', '.dylib', '.exe', '.dll', '.a', '.o', '.lockb'}:
            reason = 'compiled/database/binary file'
        if reason:
            rec.update(status='excluded', reason=reason)
            records.append(rec)
            continue
        raw = src.read_bytes()
        rec['source_sha256'] = hashlib.sha256(raw).hexdigest()
        try:
            if raw.startswith((b'\xff\xfe', b'\xfe\xff')):
                text = raw.decode('utf-16'); rec['source_encoding'] = 'utf-16'
            else:
                # NUL-delimited Git receipts and a source snapshot are valid
                # UTF-8 evidence. Decode validity, not NUL alone, distinguishes
                # these from the remaining binary files.
                text = raw.decode('utf-8')
        except UnicodeError:
            rec.update(status='excluded', reason='binary or unrecognized text encoding')
            records.append(rec)
            continue
        safe = sanitize(text).encode('utf-8')
        target = STAGE / rec['path']
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(safe)
        rec.update(status='included', published_bytes=len(safe), published_sha256=hashlib.sha256(safe).hexdigest(),
                   redacted_or_normalized=safe != raw)
        records.append(rec)

manifest = {'format':1, 'scope':'resume evidence; not new execution or release approval',
            'roots':{k:str(v) for k,v in ROOTS.items()}, 'files':records,
            'counts':dict(Counter(r['status'] for r in records)),
            'credential_value_markers':len(masked),
            'warning':'Credential-like values replaced with opaque markers. Published hashes bind sanitized copies; source hashes identify originals but do not claim remote byte identity. Local originals were not changed. Excluded binaries, visuals, DBs, caches and credentials remain explicitly listed.'}
(DEST/'MANIFEST.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
archive = DEST/'EVIDENCE.tar.gz'
with archive.open('wb') as f, gzip.GzipFile(filename='',fileobj=f,mode='wb',mtime=0) as gz, tarfile.open(fileobj=gz,mode='w') as tf:
    for rec in records:
        if rec['status'] != 'included':
            continue
        blob = (STAGE/rec['path']).read_bytes()
        entry = tarfile.TarInfo(rec['path']); entry.size=len(blob); entry.mtime=0; entry.mode=0o644
        tf.addfile(entry,io.BytesIO(blob))
summary = {**manifest['counts'], 'redacted_or_normalized':sum(r.get('redacted_or_normalized',False) for r in records),
           'archive_bytes':archive.stat().st_size,'archive_sha256':hashlib.sha256(archive.read_bytes()).hexdigest(),
           'exclusions':dict(Counter(r['reason'] for r in records if r['status']=='excluded'))}
(DEST/'PACKAGE-RESULT.json').write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps(summary,indent=2))
