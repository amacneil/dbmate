import { execSync } from "node:child_process";

import { resolveBinary } from "./resolveBinary.js";

type Args = {
  // Location of the migration files. Defaults to $(cwd)/db/migrations.
  migrationsDir?: string;
};

export class Dbmate {
  private binaryPath: string;
  private dbUrl: string;
  private args: Args | undefined;

  constructor(dbUrl: string, args?: Args) {
    this.binaryPath = resolveBinary();
    this.dbUrl = dbUrl;
    this.args = args;
  }

  async up(): Promise<void> {
    const cmd = `${this.binaryPath} --env DB_URL ${this.buildCliArgs()} up`;
    execSync(cmd, {
      env: {
        DB_URL: this.dbUrl,
      },
    });
  }

  async down(): Promise<void> {
    const cmd = `${this.binaryPath} --env DB_URL ${this.buildCliArgs()} down`;
    execSync(cmd, {
      env: {
        DB_URL: this.dbUrl,
      },
    });
  }

  async drop(): Promise<void> {
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
