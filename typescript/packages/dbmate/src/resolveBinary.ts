import { createRequire } from "node:module";
import { arch, platform } from "node:process";

const require = createRequire(import.meta.url);

/**
 * Resolve path to dbmate for the current platform
 * */
export function resolveBinary(): string {
  const ext = platform === "win32" ? ".exe" : "";
  const path = `@dbmate/${platform}-${arch}/bin/dbmate${ext}`;

  try {
    return require.resolve(path);
  } catch (err) {
    if (
      err != undefined &&
      typeof err === "object" &&
      "code" in err &&
      err.code === "MODULE_NOT_FOUND"
    ) {
      throw new Error(`Unable to locate dbmate binary '${path}'`);
    } else {
      throw err;
    }
  }
}
