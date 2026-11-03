// Runs the extension test in VS Code on a copy of examples/c4. VS Code is the one named by
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
  const workspace = path.join(tmp, "c4");
  fs.cpSync(path.resolve(root, "../examples/c4"), workspace, { recursive: true });
  const vscodeExecutablePath = process.env.DG_VSCODE ?? (fs.existsSync(insiders) ? insiders : await downloadAndUnzipVSCode());
  const extensionsDir = path.join(tmp, "extensions");
  let extensionDevelopmentPath = root;
  const launchArgs = [workspace, "--user-data-dir", path.join(tmp, "user"), "--extensions-dir", extensionsDir];
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
