#!/usr/bin/env node
import { spawnSync } from "node:child_process";

import { resolveBinary } from "./resolveBinary.js";

const child = spawnSync(resolveBinary(), process.argv.slice(2), {
  stdio: "inherit",
});
if (child.error) {
  console.error(child.error);
}
process.exit(child.status ?? 1);
