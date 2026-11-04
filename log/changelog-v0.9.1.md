# ScopeWeaver v0.9.1 — Windows PostgreSQL setup guidance

On Windows, the prebuilt release started `scopeweaver.exe`, failed to connect to PostgreSQL at the
configured endpoint `127.0.0.1:5433` (`connection refused`), exited with code 1, and `start.bat`
retried indefinitely ([#79](https://github.com/cskwork/scopeweaver/issues/79)). The release ZIP does
not contain PostgreSQL, the guide did not say so, and the example port disagreed with the standard
`5432` used by the README and installer. This release corrects the example and the guidance; it does
not bundle a database or detect ports automatically.

## Fixed

- `config.example.json` uses port `5432`. Only newly extracted and copied examples change. An existing
  `config.json` is never modified, and an explicit `5433` remains valid when PostgreSQL really listens
  there. There is no silent fallback to another port, and changing the port alone does not install or
  start PostgreSQL.
- The prebuilt-binary install section in all six README editions adds Windows PowerShell steps: stop the
  retry loop, install or start PostgreSQL separately, verify the actual port and an authenticated
  read-only query, create `config.json` only if it is absent, confirm which config source and overrides
  apply, set a tested password when password authentication is required, clear a copied Unix `skill_dir`,
  and treat health or the UI opening, not the early `Listening on` banner, as ready.
- Chat context snapshots no longer fail to save when captured content contains NUL characters:
  real `\u0000` escapes are stripped before the `jsonb` write while a literal `\\u0000` text is kept. See
  [#80](https://github.com/cskwork/scopeweaver/pull/80).

## Known security issue

The Zod advisory disclosed in v0.8.0 is unchanged and still tracked in [issue #67](https://github.com/cskwork/scopeweaver/issues/67). See the [v0.8.0 note](../log/changelog-v0.8.0.md#known-security-issue).

## Upgrade and verification

v0.9.1 adds no database migration and no schema, authentication or runtime connection change for #79.
Back up the PostgreSQL database, data directory and `config.json` before upgrading, and keep them. The
in-app update replaces only the binary: it does not refresh `config.example.json`, the README files or
an existing `config.json`, so updated installations keep their current settings.

Required before release: `go test ./config` including a regression that reads the shipped example,
six-language README parity, inspection of the Windows ZIP contents, the native Windows launcher fixture
and a Windows CI startup against a disposable native PostgreSQL `scopeweaver_test_*` database, plus all
required checks on the PR and `checks` and `e2e` on `dev` and on `main`. Native Windows recovery has not
been confirmed by the reporter.
