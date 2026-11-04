import * as cp from "node:child_process";
import * as fs from "node:fs";
import * as path from "node:path";
import * as vscode from "vscode";
import { LanguageClient } from "vscode-languageclient/node";

interface Project {
  client: LanguageClient;
  watcher: vscode.FileSystemWatcher;
}

const projects = new Map<string, Project>();
/** claimed maps each document the extension gave the language dg to the language it had. */
const claimed = new Map<string, string>();
const caseless = process.platform === "darwin" || process.platform === "win32";

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
  const projectFiles = vscode.workspace.createFileSystemWatcher("**/dg.hcl", false, true, false);
  context.subscriptions.push(
    vscode.commands.registerCommand("dg.openDiagram", () => openDiagram(dg)),
    vscode.workspace.onDidOpenTextDocument((d) => opened(dg, d)),
    vscode.workspace.onDidCloseTextDocument((d) => {
      // A change of language closes and reopens a document that stays in textDocuments.
      if (!vscode.workspace.textDocuments.includes(d)) claimed.delete(d.uri.toString());
    }),
    vscode.workspace.onDidChangeConfiguration((e) => e.affectsConfiguration("dg.projectFolders") && refresh(dg)),
    vscode.workspace.onDidChangeWorkspaceFolders(() => refresh(dg)),
    projectFiles,
    projectFiles.onDidCreate(() => refresh(dg)),
    projectFiles.onDidDelete(() => refresh(dg)),
  );
  await refresh(dg);
  return { diagramURL: () => diagram?.url };
}

/**
 * refresh stops the servers of folders that are no longer projects, gives back their language
 * to the documents it claimed for them, and handles every open document as newly opened.
 */
async function refresh(dg: string): Promise<void> {
  for (const [root, p] of projects) {
    if (isProject(root)) continue;
    projects.delete(root);
    p.watcher.dispose();
    await p.client.stop();
  }
  await Promise.all(
    vscode.workspace.textDocuments.map((doc) => {
      const key = doc.uri.toString();
      const was = claimed.get(key);
      if (doc.languageId !== "dg" || !was || projectRoot(doc.uri)) return opened(dg, doc);
      claimed.delete(key);
      return vscode.languages.setTextDocumentLanguage(doc, was);
    }),
  );
}

/**
 * opened gives a .hcl file of a dg project the language dg, if it is plain text or HCL, and
 * starts its project's server.
 */
async function opened(dg: string, doc: vscode.TextDocument): Promise<void> {
  const key = doc.uri.toString();
  const claimable = (doc.languageId === "plaintext" || doc.languageId === "hcl") && doc.uri.path.toLowerCase().endsWith(".hcl");
  // Claimed once per opening, so a language the user picks instead stays.
  if (doc.languageId !== "dg" && (!claimable || claimed.has(key))) return;
  const root = projectRoot(doc.uri);
  if (!root) return;
  if (doc.languageId === "dg") return startClient(dg, root);
  claimed.set(key, doc.languageId);
  await vscode.languages.setTextDocumentLanguage(doc, "dg");
}

function samePath(a: string, b: string): boolean {
  return caseless ? a.toLowerCase() === b.toLowerCase() : a === b;
}

function listed(folder: vscode.WorkspaceFolder): string[] {
  return vscode.workspace
    .getConfiguration("dg", folder.uri)
    .get<string[]>("projectFolders", [])
    .map((p) => path.resolve(folder.uri.fsPath, p));
}

function isProject(dir: string): boolean {
  const folder = vscode.workspace.getWorkspaceFolder(vscode.Uri.file(dir));
  return !!folder && (fs.existsSync(path.join(dir, "dg.hcl")) || listed(folder).some((p) => samePath(p, dir)));
}

/**
 * projectRoot is the nearest folder above uri, within its workspace folder, that holds dg.hcl
 * or that the setting dg.projectFolders lists.
 */
function projectRoot(uri: vscode.Uri): string | undefined {
  const folder = uri.scheme === "file" ? vscode.workspace.getWorkspaceFolder(uri) : undefined;
  if (!folder) return undefined;
  const top = folder.uri.fsPath;
  const list = listed(folder);
  for (let dir = path.dirname(uri.fsPath); ; dir = path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, "dg.hcl")) || list.some((p) => samePath(p, dir))) return dir;
    if (samePath(dir, top) || dir === path.dirname(dir)) return undefined;
  }
}

/**
 * startClient runs dg lsp for the project in root, once. Its selector also matches the files of
 * projects inside it, which the middleware keeps from it.
 */
async function startClient(dg: string, root: string): Promise<void> {
  if (projects.has(root)) return;
  const uri = vscode.Uri.file(root);
  const mine = (doc: vscode.TextDocument) => projectRoot(doc.uri) === root;
  const watcher = vscode.workspace.createFileSystemWatcher(new vscode.RelativePattern(uri, "**/*.{hcl,yaml}"));
  const client = new LanguageClient(
    "dg",
    "dg",
    { command: dg, args: ["lsp", "-C", root] },
    {
      documentSelector: [{ scheme: "file", language: "dg", pattern: { baseUri: uri.toString(), pattern: "**/*.hcl" } }],
      workspaceFolder: vscode.workspace.getWorkspaceFolder(uri),
      synchronize: { fileEvents: watcher },
      middleware: {
        didOpen: (d, next) => (mine(d) ? next(d) : Promise.resolve()),
        didChange: (e, next) => (mine(e.document) ? next(e) : Promise.resolve()),
        didSave: (d, next) => (mine(d) ? next(d) : Promise.resolve()),
        provideCompletionItem: (d, p, c, t, next) => (mine(d) ? next(d, p, c, t) : undefined),
        provideHover: (d, p, t, next) => (mine(d) ? next(d, p, t) : undefined),
        provideDefinition: (d, p, t, next) => (mine(d) ? next(d, p, t) : undefined),
        provideReferences: (d, p, o, t, next) => (mine(d) ? next(d, p, o, t) : undefined),
        prepareRename: (d, p, t, next) => (mine(d) ? next(d, p, t) : undefined),
        provideRenameEdits: (d, p, n, t, next) => (mine(d) ? next(d, p, n, t) : undefined),
        provideDocumentFormattingEdits: (d, o, t, next) => (mine(d) ? next(d, o, t) : undefined),
      },
    },
  );
  projects.set(root, { client, watcher });
  await client.start();
}

export async function deactivate(): Promise<void> {
  diagram?.panel.dispose();
  await Promise.all([...projects.values()].map((p) => p.client.stop()));
}

async function openDiagram(dg: string): Promise<void> {
  if (diagram) {
    diagram.panel.reveal(vscode.ViewColumn.Beside);
    return;
  }
  const active = vscode.window.activeTextEditor?.document.uri;
  let root = active && projectRoot(active);
  if (!root) {
    const [project] = await vscode.workspace.findFiles("**/dg.hcl", undefined, 1);
    root = project && projectRoot(project);
  }
  if (!root) {
    void vscode.window.showErrorMessage("dg: no dg project in the workspace: open a file of one first.");
    return;
  }
  // With -stdin, dg serve stops when the pipe to its standard input closes, which happens
  // however the extension host ends.
  const serve = cp.spawn(dg, ["serve", "-C", root, "-open=false", "-stdin"], { stdio: ["pipe", "pipe", "pipe"] });
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
    `Diagram: ${path.basename(root)}`,
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
