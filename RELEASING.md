# Releasing

Steps to publish a new version of dbmate.

## 1. Create Version Bump PR

Create a PR titled `vX.Y.Z` ([example](https://github.com/amacneil/dbmate/pull/662)). Most releases should be a semver minor bump (e.g. `v2.24.0` → `v2.25.0`).

**Commit 1: `vX.Y.Z`**

- Update [version.go](/pkg/dbmate/version.go) with new version number

**Commit 2: `Upgrade dependencies`**

- Update `Dockerfile` with the latest stable release of [golangci-lint](https://github.com/golangci/golangci-lint/releases)
- Run `make update-deps` to update all Go and TypeScript dependencies

## 2. Create GitHub Release

After the PR is merged, create a new release titled `vX.Y.Z` on the [releases page](https://github.com/amacneil/dbmate/releases) and write release notes.

GitHub Actions will automatically build and publish binaries, publish the NPM package, and open a Homebrew PR.
