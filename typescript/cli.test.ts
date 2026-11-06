import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import ts from "typescript";

for (const scenario of [
  { name: "success", body: "exit 0", mode: 0o755, status: 0 },
  { name: "failure", body: "exit 42", mode: 0o755, status: 42 },
  { name: "EACCES", body: "exit 0", mode: 0o644, status: 1 },
  { name: "SIGTERM", body: "kill -TERM $$", mode: 0o755, status: 1 },
]) {
  void test(scenario.name, { skip: process.platform === "win32" }, () => {
    const dir = mkdtempSync(join(tmpdir(), "dbmate-cli-"));
    try {
      writeFileSync(
        join(dir, "package.json"),
        JSON.stringify({ type: "module" }),
      );
      for (const name of ["cli", "resolveBinary"]) {
        const source = readFileSync(
          new URL(`./packages/dbmate/src/${name}.ts`, import.meta.url),
          "utf8",
        );
        writeFileSync(
          join(dir, `${name}.js`),
          ts.transpile(source, { module: ts.ModuleKind.ESNext }),
        );
      }
      const binDir = join(
        dir,
        "node_modules",
        "@dbmate",
        `${process.platform}-${process.arch}`,
        "bin",
      );
      mkdirSync(binDir, { recursive: true });
      const binary = join(binDir, "dbmate");
      writeFileSync(binary, `#!/bin/sh\n${scenario.body}\n`);
      chmodSync(binary, scenario.mode);
      const child = spawnSync(process.execPath, [join(dir, "cli.js")], {
        encoding: "utf8",
      });
      assert.equal(child.status, scenario.status);
      if (scenario.name === "EACCES") {
        assert.match(child.stderr, /EACCES/);
      }
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
}
