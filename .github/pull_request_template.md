## Related issue

Fixes #<!-- issue number -->

## Description

<!-- What does this change, and why? Call out any design decisions or trade-offs. -->

## Checklist

- [ ] Acceptance tests added or updated (`TestAcc...`), and `make testacc` passes against a local Vaultwarden (`docker compose up -d --wait`)
- [ ] Tested against every supported Vaultwarden version (`VW_VERSION`, see the matrix in `.github/workflows/test.yml`) where behavior differs
- [ ] `make generate` run, and the regenerated `docs/` committed
- [ ] `make lint` passes
- [ ] `CHANGELOG.md` updated under `## Unreleased`
