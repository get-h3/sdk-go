# Governance

This document describes how the H3 Go SDK project is governed. It is a living
document and may be updated as the project evolves.

## Roles

### Maintainers

Maintainers are responsible for:

- Reviewing and merging pull requests
- Triaging issues and feature requests
- Cutting releases and maintaining the changelog
- Enforcing the project's code of conduct
- Maintaining the project roadmap

The current maintainer list is published in `CODEOWNERS`.

### Contributors

Contributors are anyone who submits issues, pull requests, documentation
improvements, or participates in discussions. All contributions are welcome
under the terms of the `LICENSE` (MIT) and `CONTRIBUTING.md`.

## Decision Making

Decisions are made by maintainers, informed by community discussion:

1. **Feature requests** are discussed in issues. A maintainer labels a feature
   as accepted when there is clear value and a viable implementation path.
2. **Design changes** to the H3 protocol or SDK surface follow the upstream
   specification in `get-h3/h3` (`specs/04-SDK-Libraries.md`). Protocol-level
   changes are coordinated with the other H3 SDK maintainers (Python,
   TypeScript, shim).
3. **Controversial decisions** are documented in the issue thread before
   implementation.

## Code Review

- All changes land via pull request.
- Every PR must pass the GitReins quality gate (secrets, lint, build, tests)
  and CI before merge.
- At least one maintainer approval is required.
- PRs that change generated protocol types must regenerate from the OpenAPI
  source of truth in `get-h3/protocol`.

## Release Process

1. Version bumps follow semantic versioning (SemVer).
2. Releases are tagged in git and documented in `CHANGELOG.md`.
3. Before tagging, the full test suite and the `h3-test` compliance battery
   must pass against a released harness.

## Contact

- Issues: https://github.com/get-h3/sdk-go/issues
- Discussions happen in the get-h3 GitHub organization repositories.
