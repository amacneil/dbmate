# Releasing

The following steps should be followed to publish a new version of dbmate (requires write access to this repository).

1. Update [version.go](/pkg/dbmate/version.go) and [README.md](/README.md) with new version number ([example PR](https://github.com/amacneil/dbmate/pull/79/files))
2. Create new release on GitHub project [releases page](https://github.com/amacneil/dbmate/releases)
3. Build using `make docker` and upload contents of `dist/` directory as assets attached to the GitHub release
4. Create PR to update Homebrew package by running the following command:

```
$ brew bump-formula-pr --url=https://github.com/amacneil/dbmate/archive/vX.Y.Z.tar.gz dbmate
```
