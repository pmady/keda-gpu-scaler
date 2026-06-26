# Governance

How keda-gpu-scaler is run.

## Overview

Right now this is a single-maintainer project. The governance here is intentionally simple. It'll grow as the contributor base does.

## Roles

### Maintainer

Maintainers have full commit access and are responsible for:

- Reviewing and merging pull requests
- Triaging issues
- Cutting releases
- Setting project direction and roadmap
- Enforcing the Code of Conduct

Current maintainers:

| Name | GitHub | Role |
|------|--------|------|
| Pavan Madduri | [@pmady](https://github.com/pmady) | Project creator, lead maintainer |

### Contributor

Anyone who has had a pull request merged. Contributors are listed in the GitHub contributors graph and recognized in release notes when applicable.

### Reviewer

Active contributors may be granted reviewer status, meaning their approvals count toward PR review requirements. Maintainers nominate reviewers based on track record.

## Decision Making

- Day-to-day decisions (bug fixes, minor features, dependency updates) are made by the maintainer through the normal PR review process.
- Larger decisions (new scaling profiles, architecture changes, breaking API changes) are discussed in a GitHub issue or discussion before implementation. Anyone can participate.
- If there's disagreement, the maintainer makes the final call and documents the reasoning in the issue.

## Adding Maintainers

New maintainers may be added when:

1. They've been contributing consistently (code, reviews, docs, whatever)
2. They show good judgment on project direction
3. An existing maintainer nominates them
4. No objections from other maintainers after a 7-day comment period on a governance issue

## Releases

Releases follow [semantic versioning](https://semver.org/). Any maintainer can cut a release. Release notes are generated from the changelog and published as GitHub Releases.

## Changes to Governance

Propose changes via PR. Needs at least one maintainer approval. Big changes (like adding a steering committee) get discussed in an issue first.

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
