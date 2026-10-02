# ScopeWeaver provenance and modifications

[한국어](ko/PROVENANCE.md)

ScopeWeaver is a standalone derivative of [Autumn-27/ARTEX](https://github.com/Autumn-27/ARTEX), based on commit `160fe13`. It is maintained separately at [cskwork/scopeweaver](https://github.com/cskwork/scopeweaver), without a GitHub fork-network relationship.

The ScopeWeaver modifications dated **2026-10-02** cover the product name, English-default/Korean-selectable interface and messages, documentation and bundled agent guidance, setup/build/start/update/password-reset prompts, report and agent language handling, and distribution naming. The executable is `scopeweaver` (`scopeweaver.exe` on Windows); archive names follow `scopeweaver-<version>-<os>-<arch>.zip`. Docker Compose builds the checked-out source locally. The configured release workflow targets this repository and GHCR.

The original `LICENSE` is retained unchanged. Original authorship, contributor acknowledgments and upstream changelog history remain attributed to ARTEX. The Go module `github.com/Autumn-27/artex`, source entry point `./cmd/artex`, `ARTEX_*` configuration contracts, `artex` database defaults and `ARTEX` login username remain for compatibility.

This modification date is not a release tag. It does not assert that binary releases or container images have been published. Historical validation documents under `sidequestion/` describe upstream runs; they are not new ScopeWeaver verification receipts. Translated historical JSON preserves technical identifiers and measurements while labeling translated narrative.

Additional modifications on **2026-10-03** complete the server translation audit and add GLM-5.3 configuration templates plus its required always-enabled reasoning compatibility. Templates use the existing provider adapter and do not create or activate a profile automatically.
