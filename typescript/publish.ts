import { exec } from "@actions/exec";
import { readdir } from "node:fs/promises";

async function main() {
  const packages = [`./dist/dbmate`];
  (await readdir("dist/@dbmate")).forEach((pkg) =>
    packages.push(`./dist/@dbmate/${pkg}`),
  );

  for (const pkg of packages) {
    await exec("corepack", [
      "npm",
      "publish",
      "--provenance",
      "--access",
      "public",
      pkg,
    ]);
  }
}

main().catch((e: unknown) => {
  console.error(e);
  process.exit(1);
});
