import { readFile, writeFile, copyFile, chmod, mkdir } from "fs/promises";
import { parse as parseYaml } from "yaml";
import rimraf from "rimraf";
import Handlebars from "handlebars";

type MatrixItem = {
  os: string;
  arch: string;
};

type PackageJson = {
  version: string;
  optionalDependencies: Record<string, string>;
};

// map GOOS to NPM
const OS_MAP: Record<string, string> = {
  linux: "linux",
  macos: "darwin",
  windows: "win32",
};

// map GOARCH to NPM
const ARCH_MAP: Record<string, string> = {
  amd64: "x64",
  arm: "arm",
  arm64: "arm64",
};

// fetch version number
async function getVersion(): Promise<string> {
  const versionFile = await readFile("../pkg/dbmate/version.go", "utf8");
  const matches = versionFile.match(/Version = "([^"]+)"/);

  if (!matches || !matches[1]) {
    throw new Error("Unable to detect version from version.go");
  }

  return matches[1];
}

// fetch github actions build matrix
async function getBuildMatrix() {
  const contents = await readFile("../.github/workflows/ci.yml", "utf8");
  const ci = parseYaml(contents);

  return ci.jobs.build.strategy.matrix.include as MatrixItem[];
}

// copy and update template into new package
async function copyTemplate(
  filename: string,
  targetDir: string,
  vars: Record<string, string>
) {
  const source = await readFile(`packages/template/${filename}`, "utf8");
  const template = Handlebars.compile(source);
  await writeFile(`${targetDir}/${filename}`, template(vars));
}

async function main() {
  // parse root package.json template
  const version = await getVersion();
  const rootPackage: PackageJson = JSON.parse(
    await readFile("packages/dbmate/package.template.json", "utf8")
  );
  rootPackage.version = version;

  // generate npm packages
  const buildMatrix = await getBuildMatrix();
  for (const build of buildMatrix) {
    const jsOS = OS_MAP[build.os];
    if (!jsOS) {
      throw new Error(`Unknown os ${build.os}`);
    }

    const jsArch = ARCH_MAP[build.arch];
    if (!jsArch) {
      throw new Error(`Unknown arch ${build.arch}`);
    }

    const name = `@dbmate/${jsOS}-${jsArch}`;
    const targetDir = `packages/@dbmate/${jsOS}-${jsArch}`;
    const binext = jsOS === "win32" ? ".exe" : "";
    const templateVars = { jsOS, jsArch, name, version };

    // generate package directory
    console.log(`Generate ${name}`);
    await rimraf(targetDir);
    await mkdir(`${targetDir}/bin`, { recursive: true });
    await copyTemplate("package.json", targetDir, templateVars);
    await copyTemplate("README.md", targetDir, templateVars);

    // copy binary from github actions artifact
    const binfile = `${targetDir}/bin/dbmate${binext}`;
    await copyFile(
      `../dist/dbmate-${build.os}-${build.arch}/dbmate-${build.os}-${build.arch}${binext}`,
      binfile
    );
    await chmod(binfile, 0o755);

    // record dependency in root package
    rootPackage.optionalDependencies[name] = version;
  }

  // write root package.json
  await writeFile(
    "packages/dbmate/package.json",
    JSON.stringify(rootPackage, undefined, 2)
  );

  // copy readme and license
  await copyFile("../LICENSE", "packages/dbmate/LICENSE");
  await copyFile("../README.md", "packages/dbmate/README.md");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
