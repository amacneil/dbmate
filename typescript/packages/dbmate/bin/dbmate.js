#!/usr/bin/env node

const { spawnSync } = require("child_process");
const { arch, platform } = require("process");

const packageName = `@dbmate/${platform}-${arch}`;
const binName = platform === "win32" ? "dbmate.exe" : "dbmate";
let binPath = `${packageName}/bin/${binName}`;

try {
  binPath = require.resolve(binPath);
} catch (error) {
  console.error(`Error: Unable to locate package ${packageName}`);
  process.exit(1);
}

const child = spawnSync(binPath, process.argv.slice(2), { stdio: "inherit" });
process.exit(child.status);
