# Releasing

The following steps should be followed to publish a new version of dbmate (requires write access to this repository).

1. Update [version.go](/pkg/dbmate/version.go) with new version number ([example PR](https://github.com/amacneil/dbmate/pull/146/files))
2. Create new release on [releases page](https://github.com/amacneil/dbmate/releases) and write release notes
3. GitHub Actions will do the rest (publish binaries, NPM package, and Homebrew PR)
