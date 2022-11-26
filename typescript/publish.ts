import { readdir } from "fs/promises";
import { exec } from "@actions/exec";

async function main() {
  const args = ["publish", "--access", "public"];
  await exec("npm", args.concat([`./packages/dbmate`]));

  for (const pkg of await readdir("packages/@dbmate")) {
    await exec("npm", args.concat([`./packages/@dbmate/${pkg}`]));
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
