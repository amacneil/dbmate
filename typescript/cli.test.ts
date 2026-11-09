import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

// The dbmate package as it is published, built by `npm run generate`.
const packageDir = fileURLToPath(new URL("./dist/dbmate", import.meta.url));

for (const scenario of [
  { name: "success", body: "exit 0", mode: 0o755, status: 0 },
  { name: "failure", body: "exit 42", mode: 0o755, status: 42 },
  { name: "EACCES", body: "exit 0", mode: 0o644, status: 1 },
  { name: "SIGTERM", body: "kill -TERM $$", mode: 0o755, status: 1 },
]) {
  void test(scenario.name, { skip: process.platform === "win32" }, () => {
    assert.ok(
      existsSync(join(packageDir, "dist", "cli.js")),
      `${packageDir} not found, run \`npm run generate\` first`,
    );

    const dir = mkdtempSync(join(tmpdir(), "dbmate-cli-"));
    try {
      cpSync(join(packageDir, "package.json"), join(dir, "package.json"));
      cpSync(join(packageDir, "dist"), join(dir, "dist"), { recursive: true });

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

      const child = spawnSync(process.execPath, [join(dir, "dist", "cli.js")], {
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
