#!/usr/bin/env node
// The MCPB bundle's server: it runs the installed app's own MCP server,
// `bowel mcp`, which carries stdio to the running app (app_mcp.go). All this
// adds is finding the program, as an MCP client starts it with a short PATH.
"use strict";

const { spawn } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

// Where bowel's installers put the program, after the path the user gave.
function candidates() {
  const home = os.homedir();
  const list = [];
  const own = process.env.BOWEL_PATH;
  // An unset user_config value can arrive empty or as the placeholder itself.
  if (own && !own.includes("${")) list.push(own);
  switch (process.platform) {
    case "darwin":
      list.push(
        "/Applications/bowel.app/Contents/MacOS/bowel",
        path.join(home, "Applications", "bowel.app", "Contents", "MacOS", "bowel"),
      );
      break;
    case "win32":
      list.push(
        path.join(process.env.LOCALAPPDATA || path.join(home, "AppData", "Local"), "Programs", "bowel", "bowel.exe"),
      );
      break;
    default:
      // The deb and rpm packages, then install.sh's user install.
      list.push("/opt/bowel/bowel", path.join(home, ".local", "bowel.app", "bowel"));
  }
  return list;
}

function isFile(p) {
  try {
    return fs.statSync(p).isFile();
  } catch {
    return false;
  }
}

const program = candidates().find(isFile) || "bowel";
const child = spawn(program, ["mcp"], { stdio: "inherit", windowsHide: true });

child.on("error", (err) => {
  process.stderr.write(
    `bowel: cannot run ${program} (${err.message}). Install bowel from ` +
      "https://github.com/21stware/bowel/releases/latest, or set where it is in this extension's settings.\n",
  );
  process.exit(1);
});
child.on("exit", (code, signal) => process.exit(signal ? 1 : (code ?? 0)));
for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => child.kill(sig));
