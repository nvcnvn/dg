// Runs the extension test in VS Code on a folder that holds a copy of examples/c4 with a copy of
// examples/hello inside it, a copy of examples/hello without dg.hcl, and a folder with a .hcl file
// of no dg project. VS Code is the one named by
// DG_VSCODE, else VS Code Insiders where macOS installs it, else a build @vscode/test-electron
// downloads. With DG_VSIX set to a .vsix file, the test runs that package installed, rather than
// this directory as a development extension.
import * as cp from "node:child_process";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { downloadAndUnzipVSCode, resolveCliArgsFromVSCodeExecutablePath, runTests } from "@vscode/test-electron";

const insiders = "/Applications/Visual Studio Code - Insiders.app/Contents/MacOS/Code - Insiders";

async function main(): Promise<void> {
  // Started from a terminal inside VS Code, the environment would make VS Code run as Node or
  // talk to the running instance.
  for (const name of Object.keys(process.env)) {
    if (name.startsWith("VSCODE_") || name === "ELECTRON_RUN_AS_NODE") delete process.env[name];
  }
  const root = path.resolve(__dirname, "../..");
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dg-vscode-"));
  const workspace = path.join(tmp, "workspace");
  fs.cpSync(path.resolve(root, "../examples/c4"), path.join(workspace, "c4"), { recursive: true });
  fs.cpSync(path.resolve(root, "../examples/hello"), path.join(workspace, "nodg"), { recursive: true });
  fs.rmSync(path.join(workspace, "nodg", "dg.hcl"));
  fs.cpSync(path.resolve(root, "../examples/hello"), path.join(workspace, "c4", "nested"), { recursive: true });
  fs.mkdirSync(path.join(workspace, "other"));
  fs.writeFileSync(path.join(workspace, "other", "main.hcl"), 'resource "aws_s3_bucket" "logs" {\n  bucket = "logs"\n}\n');
  const vscodeExecutablePath = process.env.DG_VSCODE ?? (fs.existsSync(insiders) ? insiders : await downloadAndUnzipVSCode());
  const extensionsDir = path.join(tmp, "extensions");
  let extensionDevelopmentPath = root;
  // The DevTools port lets the test send input to the diagram panel's frame as a user would.
  const userData = path.join(tmp, "user");
  // VS Code's chat and agents views open beside the editors and move them while the test runs.
  fs.mkdirSync(path.join(userData, "User"), { recursive: true });
  fs.writeFileSync(
    path.join(userData, "User", "settings.json"),
    JSON.stringify({ "chat.disableAIFeatures": true, "workbench.secondarySideBar.defaultVisibility": "hidden", "workbench.startupEditor": "none" }),
  );
  const launchArgs = [workspace, "--user-data-dir", userData, "--extensions-dir", extensionsDir, "--remote-debugging-port=0"];
  const vsix = process.env.DG_VSIX;
  if (vsix) {
    const [cli, ...args] = resolveCliArgsFromVSCodeExecutablePath(vscodeExecutablePath, { reuseMachineInstall: true });
    cp.execFileSync(cli, [...args, "--extensions-dir", extensionsDir, "--install-extension", path.resolve(vsix)], { stdio: "inherit" });
    extensionDevelopmentPath = path.join(tmp, "stub");
    fs.mkdirSync(extensionDevelopmentPath);
    fs.writeFileSync(
      path.join(extensionDevelopmentPath, "package.json"),
      JSON.stringify({ name: "stub", publisher: "test", version: "0.0.0", engines: { vscode: "*" } }),
    );
  } else {
    launchArgs.push("--disable-extensions");
  }
  try {
    await runTests({
      vscodeExecutablePath,
      extensionDevelopmentPath,
      extensionTestsPath: path.join(__dirname, "suite.js"),
      extensionTestsEnv: { DG_USER_DATA: userData },
      launchArgs,
    });
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
