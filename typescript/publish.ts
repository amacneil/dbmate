import { exec } from "@actions/exec";
import { readdir } from "node:fs/promises";

async function main() {
  const packages = [`./dist/dbmate`];
  (await readdir("dist/@dbmate")).forEach((pkg) =>
    packages.push(`./dist/@dbmate/${pkg}`),
  );

  for (const pkg of packages) {
    // Unset NODE_AUTH_TOKEN to avoid conflicts with OIDC trusted publishing
    delete process.env.NODE_AUTH_TOKEN;
    await exec("corepack", ["npm", "--version"]);
    await exec("corepack", [
      "npm",
      "publish",
      "--dry-run",
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
