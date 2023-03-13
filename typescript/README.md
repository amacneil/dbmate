# Dbmate NPM package

This directory contains scripts to generate and publish the dbmate npm package.

## Generate the package

```
npm run generate
```

For local development, you can avoid copying the dbmate binaries if you don't have them available:

```
npm run generate -- --skip-bin
```

## Publish the packages (CI only)

```
npm run publish
```
