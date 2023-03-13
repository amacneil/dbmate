import { readdir } from "fs/promises";
import { exec } from "@actions/exec";

async function main() {
  const packages = [`./dist/dbmate`];
  (await readdir("dist/@dbmate")).forEach((pkg) =>
    packages.push(`./dist/@dbmate/${pkg}`)
  );

  for (const pkg of packages) {
    await exec("npm", ["publish", "--access", "public", pkg]);
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
