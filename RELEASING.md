# Releasing

The following steps should be followed to publish a new version of dbmate (requires write access to this repository).

1. Update [version.go](/pkg/dbmate/version.go) with new version number ([example PR](https://github.com/amacneil/dbmate/pull/146/files))
2. Create new release on GitHub project [releases page](https://github.com/amacneil/dbmate/releases)
3. Travis CI will automatically publish release binaries to GitHub
4. GitHub Actions will automatically create PR to update Homebrew package
