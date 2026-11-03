import * as cp from "node:child_process";
import * as fs from "node:fs";
import * as path from "node:path";
import * as vscode from "vscode";
import { LanguageClient } from "vscode-languageclient/node";

const clients = new Map<string, LanguageClient>();

interface Diagram {
  panel: vscode.WebviewPanel;
  serve: cp.ChildProcess;
  url: string;
}

let diagram: Diagram | undefined;

/** API is what activate returns; the extension test reads the diagram server's address from it. */
export interface API {
  diagramURL(): string | undefined;
}

export async function activate(context: vscode.ExtensionContext): Promise<API> {
  const dg = context.asAbsolutePath(path.join("bin", process.platform === "win32" ? "dg.exe" : "dg"));
  context.subscriptions.push(
    vscode.commands.registerCommand("dg.openDiagram", () => openDiagram(dg)),
    vscode.workspace.onDidChangeWorkspaceFolders(async (e) => {
      for (const folder of e.removed) {
        await clients.get(folder.uri.toString())?.stop();
        clients.delete(folder.uri.toString());
      }
      await Promise.all(e.added.filter(isProject).map((f) => startClient(dg, f)));
    }),
  );
  await Promise.all(projectFolders().map((f) => startClient(dg, f)));
  return { diagramURL: () => diagram?.url };
}

/** startClient runs dg lsp for the project in folder. */
async function startClient(dg: string, folder: vscode.WorkspaceFolder): Promise<void> {
  const client = new LanguageClient(
    "dg",
    "dg",
    { command: dg, args: ["lsp", "-C", folder.uri.fsPath] },
    {
      documentSelector: [{ scheme: "file", language: "hcl", pattern: { baseUri: folder.uri.toString(), pattern: "**/*.hcl" } }],
      workspaceFolder: folder,
      synchronize: {
        fileEvents: vscode.workspace.createFileSystemWatcher(new vscode.RelativePattern(folder, "**/*.{hcl,yaml}")),
      },
    },
  );
  clients.set(folder.uri.toString(), client);
  await client.start();
}

export async function deactivate(): Promise<void> {
  diagram?.panel.dispose();
  await Promise.all([...clients.values()].map((c) => c.stop()));
}

function isProject(folder: vscode.WorkspaceFolder): boolean {
  return fs.existsSync(path.join(folder.uri.fsPath, "dg.hcl"));
}

function projectFolders(): vscode.WorkspaceFolder[] {
  return (vscode.workspace.workspaceFolders ?? []).filter(isProject);
}

async function openDiagram(dg: string): Promise<void> {
  if (diagram) {
    diagram.panel.reveal(vscode.ViewColumn.Beside);
    return;
  }
  const folders = projectFolders();
  const active = vscode.window.activeTextEditor?.document.uri;
  const folder = folders.find((f) => active && vscode.workspace.getWorkspaceFolder(active) === f) ?? folders[0];
  if (!folder) {
    void vscode.window.showErrorMessage("dg: no workspace folder contains dg.hcl.");
    return;
  }
  // With -stdin, dg serve stops when the pipe to its standard input closes, which happens
  // however the extension host ends.
  const serve = cp.spawn(dg, ["serve", "-C", folder.uri.fsPath, "-open=false", "-stdin"], { stdio: ["pipe", "pipe", "pipe"] });
  let url: string;
  try {
    url = await served(serve);
  } catch (err) {
    serve.kill();
    void vscode.window.showErrorMessage(`dg: cannot open the diagram: ${(err as Error).message}`);
    return;
  }
  const external = (await vscode.env.asExternalUri(vscode.Uri.parse(url))).toString(true);
  const panel = vscode.window.createWebviewPanel(
    "dg.diagram",
    `Diagram: ${folder.name}`,
    { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
    { enableScripts: true, retainContextWhenHidden: true },
  );
  panel.webview.html = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; frame-src ${new URL(external).origin}">
<style>html, body, iframe { margin: 0; padding: 0; border: 0; width: 100%; height: 100%; overflow: hidden; display: block; }</style>
</head>
<body><iframe src="${external}" title="Diagram"></iframe></body>
</html>`;
  diagram = { panel, serve, url };
  panel.onDidDispose(() => {
    serve.kill();
    diagram = undefined;
  });
}

/** served resolves to the address dg serve prints when it listens. */
function served(serve: cp.ChildProcess): Promise<string> {
  return new Promise((resolve, reject) => {
    let out = "";
    let errText = "";
    serve.stdout?.on("data", (data: Buffer) => {
      out += data.toString();
      const m = /dg serve: (http:\/\/\S+)/.exec(out);
      if (m) resolve(m[1]);
    });
    serve.stderr?.on("data", (data: Buffer) => (errText += data.toString()));
    serve.on("error", reject);
    serve.on("exit", (code) => reject(new Error(`dg serve exited with code ${code}. ${errText}`.trim())));
  });
}
