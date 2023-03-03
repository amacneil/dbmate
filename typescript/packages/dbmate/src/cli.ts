#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { getBinaryPath } from "./resolve";

const binPath = getBinaryPath();
const child = spawnSync(binPath, process.argv.slice(2), { stdio: "inherit" });
process.exit(child.status ?? 0);
