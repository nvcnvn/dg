import * as assert from "node:assert/strict";
import * as vscode from "vscode";
import type { API } from "../src/extension";

async function waitFor<T>(what: string, check: () => T | undefined | false, ms = 20000): Promise<T> {
  const deadline = Date.now() + ms;
  for (;;) {
    const v = check();
    if (v) return v;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 50));
  }
}

export async function run(): Promise<void> {
  const ext = vscode.extensions.getExtension<API>("nvcnvn.dg");
  assert.ok(ext, "the dg extension is installed");
  const api = await ext.activate();
  const folder = vscode.workspace.workspaceFolders![0].uri;
  const model = vscode.Uri.joinPath(folder, "model.hcl");
  const doc = await vscode.workspace.openTextDocument(model);
  assert.equal(doc.languageId, "hcl");
  const editor = await vscode.window.showTextDocument(doc);

  // An unsaved edit gets a diagnostic from dg lsp; editing it back clears the diagnostic.
  const at = doc.getText().indexOf("parent = node.banking") + "parent = node.".length;
  const start = doc.positionAt(at);
  await editor.edit((e) => e.replace(new vscode.Range(start, start.translate(0, "banking".length)), "nowhere"));
  const diags = await waitFor("a diagnostic for the unsaved edit", () => {
    const ds = vscode.languages.getDiagnostics(model);
    return ds.length > 0 && ds;
  });
  assert.match(diags[0].message, /"nowhere" .* not defined/);
  assert.equal(diags[0].range.start.line, start.line);
  await editor.edit((e) => e.replace(new vscode.Range(start, start.translate(0, "nowhere".length)), "banking"));
  await waitFor("the diagnostic to clear", () => vscode.languages.getDiagnostics(model).length === 0);

  const list = await vscode.commands.executeCommand<vscode.CompletionList>("vscode.executeCompletionItemProvider", model, start);
  assert.ok(
    list.items.some((i) => i.label === "banking"),
    "node IDs are completed after node.",
  );

  // Open diagram shows the app dg serve serves in a panel beside the editor.
  await vscode.commands.executeCommand("dg.openDiagram");
  const url = api.diagramURL();
  assert.ok(url, "dg serve is running");
  await waitFor("the diagram panel", () =>
    vscode.window.tabGroups.all.some((g) =>
      g.tabs.some((t) => t.input instanceof vscode.TabInputWebview && t.input.viewType.endsWith("dg.diagram")),
    ),
  ).catch((err: Error) => {
    const tabs = vscode.window.tabGroups.all.flatMap((g) => g.tabs.map((t) => `${t.label} ${JSON.stringify(t.input)}`));
    throw new Error(`${err.message}; tabs: ${tabs.join(", ")}`);
  });
  const page = await (await fetch(url)).text();
  assert.match(page, /id="paper"/);

  // Saving the model file sends the panel a new drawing within 300 milliseconds (FR-18).
  const ws = new WebSocket(url.replace(/^http/, "ws") + "ws");
  const snapshots: number[] = [];
  ws.onmessage = (ev) => {
    if (JSON.parse(String(ev.data)).type === "snapshot") snapshots.push(Date.now());
  };
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
  });
  ws.send(JSON.stringify({ type: "select-view", view: "main" }));
  await waitFor("the first snapshot", () => snapshots.length > 0);
  const label = doc.getText().indexOf("Personal Banking Customer");
  await editor.edit((e) => e.insert(doc.positionAt(label), "Our "));
  const saved = Date.now();
  await doc.save();
  const updated = await waitFor("a snapshot after saving", () => snapshots.length > 1 && snapshots[1]);
  ws.close();
  console.log(`dg: the panel's server sent a new drawing ${updated - saved} ms after the save`);
  assert.ok(updated - saved < 300, `the panel updated ${updated - saved} ms after the save`);
}
