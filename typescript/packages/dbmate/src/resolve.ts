import { arch, platform } from "node:process";

const packageName = `@dbmate/${platform}-${arch}`;
const binName = platform === "win32" ? "dbmate.exe" : "dbmate";
let binPath = `${packageName}/bin/${binName}`;

/** Return the binary path to the binary for the current platform */
export function getBinaryPath() {
  try {
    return require.resolve(binPath);
  } catch (err) {
    console.error(err);
    throw new Error(`Error: Unable to locate package ${packageName}`);
  }
}
