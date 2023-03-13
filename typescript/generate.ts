import { exec } from "@actions/exec";
import Handlebars from "handlebars";
import { readFile, writeFile, cp, chmod, mkdir } from "node:fs/promises";
import { parse as parseYaml } from "yaml";

type CiYaml = {
  jobs: {
    build: {
      strategy: {
        matrix: {
          include: MatrixItem[];
        };
      };
    };
  };
};

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
  386: "ia32",
  amd64: "x64",
  arm: "arm",
  arm64: "arm64",
};

// fetch version number
async function getVersion(): Promise<string> {
  const versionFile = await readFile("../pkg/dbmate/version.go", "utf8");
  const matches = versionFile.match(/Version = "([^"]+)"/);

  if (matches?.[1]) {
    return matches[1];
  }

  throw new Error("Unable to detect version from version.go");
}

// fetch github actions build matrix
async function getBuildMatrix() {
  const contents = await readFile("../.github/workflows/ci.yml", "utf8");
  const ci = parseYaml(contents) as CiYaml;

  return ci.jobs.build.strategy.matrix.include;
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
  // clean output directories
  await exec("npm", ["run", "clean"]);

  // build main package
  await exec("npm", ["run", "build"], {
    cwd: "packages/dbmate",
  });

  // parse main package.json
  const version = await getVersion();
  const mainPackageJson = JSON.parse(
    await readFile("packages/dbmate/package.json", "utf8")
  ) as PackageJson;
  mainPackageJson.version = version;

  // generate os/arch packages
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
    const targetDir = `dist/@dbmate/${jsOS}-${jsArch}`;
    const binext = jsOS === "win32" ? ".exe" : "";
    const templateVars = { jsOS, jsArch, name, version };

    // generate package directory
    console.log(`Generate ${name}`);
    await mkdir(`${targetDir}/bin`, { recursive: true });
    await copyTemplate("package.json", targetDir, templateVars);
    await copyTemplate("README.md", targetDir, templateVars);

    // copy binary from github actions artifact
    const targetBin = `${targetDir}/bin/dbmate${binext}`;
    try {
      if (process.argv[2] === "--skip-bin") {
        // dummy file for testing
        await writeFile(targetBin, "");
      } else {
        // copy os/arch binary (typically built via CI)
        await cp(
          `../dist/dbmate-${build.os}-${build.arch}/dbmate-${build.os}-${build.arch}${binext}`,
          targetBin
        );
      }
      await chmod(targetBin, 0o755);
    } catch (e) {
      console.error(e);
      throw new Error(
        "Run `npm run generate -- --skip-bin` to test generate without binaries"
      );
    }

    // record dependency in main package.json
    mainPackageJson.optionalDependencies[name] = version;
  }

  // copy main package
  await cp("packages/dbmate", "dist/dbmate", {
    recursive: true,
  });

  // write package.json
  await writeFile(
    "dist/dbmate/package.json",
    JSON.stringify(mainPackageJson, undefined, 2)
  );

  // copy readme and license
  await cp("../LICENSE", "dist/dbmate/LICENSE");
  await cp("../README.md", "dist/dbmate/README.md");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
