import { execSync } from "node:child_process";

import { getBinaryPath } from "./resolve";

type Args = {
  // Location of the migration files. Defaults to $(cwd)/db/migrations.
  migrationsDir?: string;
};

class DbMate {
  private binaryPath: string;
  private dbUrl: string;
  private args: Args | undefined;

  constructor(dbUrl: string, args?: Args) {
    this.binaryPath = getBinaryPath();
    this.dbUrl = dbUrl;
    this.args = args;
  }

  async up() {
    const cmd = `${this.binaryPath} --env DB_URL ${this.buildCliArgs()} up`;
    execSync(cmd, {
      env: {
        DB_URL: this.dbUrl,
      },
    });
  }

  async down() {
    const cmd = `${this.binaryPath} --env DB_URL ${this.buildCliArgs()} down`;
    execSync(cmd, {
      env: {
        DB_URL: this.dbUrl,
      },
    });
  }

  async drop() {
    const cmd = `${this.binaryPath} --env DB_URL ${this.buildCliArgs()} drop`;
    execSync(cmd, {
      env: {
        DB_URL: this.dbUrl,
      },
    });
  }

  private buildCliArgs(): string {
    // additional cli args
    const cliArgs = [];
    if (this.args?.migrationsDir != undefined) {
      cliArgs.push(`-d "${this.args.migrationsDir}"`);
    }

    return cliArgs.join(" ");
  }
}

export default DbMate;
