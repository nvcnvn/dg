import * as assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import * as vscode from "vscode";
import type { API } from "../src/extension";

async function waitFor<T>(what: string, check: () => T | undefined | false | Promise<T | undefined | false>, ms = 20000): Promise<T> {
  const deadline = Date.now() + ms;
  for (;;) {
    const v = await check();
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
  console.log(`dg: the panel's server sent a new drawing ${updated - saved} ms after the save`);
  assert.ok(updated - saved < 300, `the panel updated ${updated - saved} ms after the save`);

  ws.close();

  // The panel edits the layout (FR-18): a drag in the panel's frame writes the layout file, which
  // the open text editor shows, and Ctrl/Cmd+Z pressed in the frame restores it. The input goes
  // through VS Code's window as a user's would, by the DevTools protocol on the port run.ts opens.
  const layout = vscode.Uri.joinPath(folder, "views", "main.layout.yaml");
  const layoutDoc = await vscode.workspace.openTextDocument(layout);
  const original = layoutDoc.getText();
  const port = fs.readFileSync(path.join(process.env.DG_USER_DATA!, "DevToolsActivePort"), "utf8").split("\n")[0];
  const targets = async () => (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()) as { type: string; url: string; webSocketDebuggerUrl: string }[];
  const frameTarget = await waitFor("the panel's frame", async () => (await targets()).find((t) => t.url.startsWith(url)));
  const pageTarget = (await targets()).find((t) => t.type === "page" && t.url.includes("workbench"));
  assert.ok(pageTarget, "VS Code's window is a DevTools target");
  const frame = await devtools(frameTarget.webSocketDebuggerUrl);
  const win = await devtools(pageTarget.webSocketDebuggerUrl);
  const evaluate = async <T>(on: typeof win, expression: string): Promise<T> => ((await on.send("Runtime.evaluate", { expression, returnByValue: true })) as { result: { value: T } }).result.value;
  await waitFor("the drawing in the panel", () =>
    evaluate<boolean>(frame, `document.body.classList.contains('editable') && document.querySelector('[data-node="customer"]') !== null`),
  );
  const node = await evaluate<[number, number]>(
    frame,
    `(() => { const el = document.querySelector('[data-node="customer"]'); el.scrollIntoView({ block: 'center', inline: 'center' }); const r = el.getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()`,
  );
  const panel = await evaluate<[number, number]>(
    win,
    `(() => { const f = [...document.querySelectorAll('iframe.webview')].find((f) => f.getBoundingClientRect().width > 0); const r = f.getBoundingClientRect(); return [r.x, r.y]; })()`,
  );
  const [x, y] = [panel[0] + node[0], panel[1] + node[1]];
  const mouse = (type: string, dx: number, buttons: number) =>
    win.send("Input.dispatchMouseEvent", { type, x: x + dx, y, button: type === "mouseMoved" && !buttons ? "none" : "left", buttons, clickCount: 1 });
  await mouse("mouseMoved", 0, 0);
  await mouse("mousePressed", 0, 1);
  for (let dx = 10; dx <= 60; dx += 10) await mouse("mouseMoved", dx, 1);
  await mouse("mouseReleased", 60, 0);
  await waitFor("the text editor to show the moved node", () => layoutDoc.getText() !== original && layoutDoc.getText().includes("customer: {x: 310,"));
  await waitFor("the panel to draw the moved node", () => evaluate<boolean>(frame, `document.body.classList.contains('editable') && !document.getElementById('undo').disabled`));
  const mod = process.platform === "darwin" ? 4 : 2;
  for (const type of ["rawKeyDown", "keyUp"]) {
    await win.send("Input.dispatchKeyEvent", { type, modifiers: mod, key: "z", code: "KeyZ", windowsVirtualKeyCode: 90, nativeVirtualKeyCode: 90 });
  }
  await waitFor("the text editor to show the layout before the move", () => layoutDoc.getText() === original);
  frame.close();
  win.close();
}

/** devtools connects to a DevTools protocol target. */
async function devtools(wsURL: string) {
  const ws = new WebSocket(wsURL);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
  });
  let id = 0;
  const pending = new Map<number, (m: { result?: unknown; error?: { message: string } }) => void>();
  ws.onmessage = (ev) => {
    const m = JSON.parse(String(ev.data));
    pending.get(m.id)?.(m);
  };
  return {
    send(method: string, params: object): Promise<unknown> {
      return new Promise((resolve, reject) => {
        pending.set(++id, (m) => (m.error ? reject(new Error(`${method}: ${m.error.message}`)) : resolve(m.result)));
        ws.send(JSON.stringify({ id, method, params }));
      });
    },
    close: () => ws.close(),
  };
}
