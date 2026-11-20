// Generates the TypeScript bindings for the Go bridge. The Go package
// embeds frontend/dist, so the directory must exist before Go loads it.
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const frontend = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(frontend, "dist");
mkdirSync(dist, { recursive: true });
if (!existsSync(join(dist, "index.html"))) {
  writeFileSync(join(dist, ".keep"), "");
}
execFileSync(
  "go",
  ["tool", "wails3", "generate", "bindings", "-ts", "-i", "-f", "-tags=purego", "-d", "frontend/bindings", "."],
  { cwd: join(frontend, ".."), stdio: "inherit" },
);
