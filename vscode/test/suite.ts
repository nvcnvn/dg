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
  const top = vscode.workspace.workspaceFolders![0].uri;
  const folder = vscode.Uri.joinPath(top, "c4");
  const model = vscode.Uri.joinPath(folder, "model.hcl");
  const doc = await vscode.workspace.openTextDocument(model);
  await waitFor("the language dg for a file of a project with dg.hcl", () => doc.languageId === "dg");
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
  await vscode.commands.executeCommand("dg.openDiagram", model);
  assert.equal(api.diagramURL(), url, "the editor title button on a file of the same project keeps its panel");

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
  // Chromium sends no input into the panel's frame while the window is behind another one, and
  // VS Code goes on laying out its window for a while after the panel opens, so the window is
  // brought to the front, the frame is measured once it holds still, and a press the frame did
  // not receive is tried again from a new measurement.
  await win.send("Page.bringToFront", {});
  await evaluate<number>(frame, `window.pressed = 0; document.addEventListener('pointerdown', () => window.pressed++, true); 0`);
  // The view opens fitted to the panel; at 100% a pixel of the drag is a unit of the layout.
  await evaluate<number>(frame, `document.getElementById('zoom').click(); [...document.querySelectorAll('#tool-menu button')].find((b) => b.textContent === '100%').click(); 0`);
  const panelAt = () =>
    evaluate<[number, number]>(
      win,
      `(() => { const f = [...document.querySelectorAll('iframe.webview')].find((f) => f.getBoundingClientRect().width > 0); const r = f.getBoundingClientRect(); return [r.x, r.y]; })()`,
    );
  let [x, y] = [0, 0];
  const mouse = (type: string, dx: number, buttons: number) =>
    win.send("Input.dispatchMouseEvent", { type, x: x + dx, y, button: type === "mouseMoved" && !buttons ? "none" : "left", buttons, clickCount: 1 });
  for (let attempt = 1; ; attempt++) {
    let panel = await panelAt();
    for (let still = 0; still < 3; ) {
      await new Promise((r) => setTimeout(r, 200));
      const now = await panelAt();
      still = now[0] === panel[0] && now[1] === panel[1] ? still + 1 : 0;
      panel = now;
    }
    const node = await evaluate<[number, number]>(
      frame,
      `(() => { const el = document.querySelector('[data-node="customer"]'); el.scrollIntoView({ block: 'center', inline: 'center' }); const r = el.getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()`,
    );
    [x, y] = [panel[0] + node[0], panel[1] + node[1]];
    await mouse("mouseMoved", 0, 0);
    await mouse("mousePressed", 0, 1);
    if (await evaluate<number>(frame, "window.pressed")) break;
    await mouse("mouseReleased", 0, 0);
    if (attempt === 5) throw new Error("the panel's frame never received the press");
  }
  for (let dx = 10; dx <= 60; dx += 10) await mouse("mouseMoved", dx, 1);
  await mouse("mouseReleased", 60, 0);
  await waitFor("the text editor to show the moved node", () => layoutDoc.getText() !== original && layoutDoc.getText().includes("customer: {x: 310,"));
  await waitFor("the panel to draw the moved node", () => evaluate<boolean>(frame, `document.body.classList.contains('editable') && document.getElementById('undo').ariaDisabled === 'false'`));
  const mod = process.platform === "darwin" ? 4 : 2;
  for (const type of ["rawKeyDown", "keyUp"]) {
    await win.send("Input.dispatchKeyEvent", { type, modifiers: mod, key: "z", code: "KeyZ", windowsVirtualKeyCode: 90, nativeVirtualKeyCode: 90 });
  }
  await waitFor("the text editor to show the layout before the move", () => layoutDoc.getText() === original);

  // The panel edits the model (FR-18): a label edited in place in the panel is written to the
  // model file, which the open text editor shows.
  await waitFor("the panel to draw the layout before the move", () => evaluate<boolean>(frame, `document.body.classList.contains('editable') && document.getElementById('redo').ariaDisabled === 'false'`));
  const box = await evaluate<[number, number]>(
    frame,
    `(() => { const r = document.querySelector('[data-node="customer"]').getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()`,
  );
  const panel = await panelAt();
  const [cx, cy] = [panel[0] + box[0], panel[1] + box[1]];
  for (const clickCount of [1, 2]) {
    for (const type of ["mousePressed", "mouseReleased"]) await win.send("Input.dispatchMouseEvent", { type, x: cx, y: cy, button: "left", buttons: type === "mousePressed" ? 1 : 0, clickCount });
  }
  await waitFor("the label editor in the panel", () => evaluate<boolean>(frame, `document.activeElement?.classList.contains('dg-inline') === true`));
  await win.send("Input.insertText", { text: "Customer" });
  for (const type of ["rawKeyDown", "char", "keyUp"]) await win.send("Input.dispatchKeyEvent", { type, key: "Enter", code: "Enter", text: "\r", windowsVirtualKeyCode: 13, nativeVirtualKeyCode: 13 });
  await waitFor("the text editor to show the new label", () => doc.getText().includes('label = "Customer"\n'));

  // The page follows VS Code's theme, and a click on a diagnostic opens its file at its position
  // (FR-20).
  const theme = (kind: string) => waitFor(`the ${kind} theme in the panel`, async () => (await evaluate<string>(frame, `document.documentElement.dataset.theme`)) === kind);
  const workbench = vscode.workspace.getConfiguration("workbench");
  for (const [name, kind] of [["Default Light Modern", "light"], ["Default Dark Modern", "dark"], ["Default High Contrast", "high-contrast"]]) {
    await workbench.update("colorTheme", name, vscode.ConfigurationTarget.Global);
    await theme(kind);
  }
  await workbench.update("colorTheme", undefined, vscode.ConfigurationTarget.Global);
  const end = doc.lineCount;
  await editor.edit((e) => e.insert(doc.lineAt(end - 1).range.end, '\nnode "bad" {\n  kind = "nope"\n}\n'));
  await doc.save();
  await waitFor("the diagnostic in the panel", () => evaluate<boolean>(frame, `document.querySelector('#diagnostics .diags li.error button') !== null`));
  await evaluate<number>(frame, `document.querySelector('#diagnostics .diags li.error button').click(); 0`);
  const opened = await waitFor("the file of the diagnostic", () => {
    const e = vscode.window.activeTextEditor;
    return e?.document.uri.toString() === model.toString() && e.selection.active.line >= end ? e : undefined;
  });
  assert.equal(opened.document.lineAt(opened.selection.active.line).text.trim(), 'node "bad" {');
  await opened.edit((e) => e.delete(new vscode.Range(new vscode.Position(end - 1, doc.lineAt(end - 1).text.length), doc.lineAt(doc.lineCount - 1).range.end)));
  await doc.save();
  await waitFor("the panel without the diagnostic", () => evaluate<boolean>(frame, `document.body.classList.contains('editable')`));
  frame.close();
  win.close();

  // Only the .hcl files of dg projects get the language dg and its server: a project is a folder
  // with dg.hcl or one that dg.projectFolders lists. The extension handles documents in the order
  // they open, so once a later file of c4 is dg, it has passed over the earlier ones.
  const open = (uri: vscode.Uri) => vscode.workspace.openTextDocument(uri);
  const barrier = async (file: string) => {
    const d = await open(vscode.Uri.joinPath(folder, file));
    await waitFor(`the language dg for ${file}`, () => d.languageId === "dg");
    return d;
  };
  const completes = async (uri: vscode.Uri, before: string) => {
    const d = await open(uri);
    const at = d.positionAt(d.getText().indexOf(before) + before.length);
    return (await vscode.commands.executeCommand<vscode.CompletionList>("vscode.executeCompletionItemProvider", uri, at)).items.map((i) => i.label);
  };
  const other = vscode.Uri.joinPath(top, "other", "main.hcl");
  const otherDoc = await open(other);
  const otherLanguage = otherDoc.languageId;
  const hello = vscode.Uri.joinPath(top, "nodg", "model.hcl");
  const helloDoc = await open(hello);
  const helloLanguage = helloDoc.languageId;
  const helloStyle = await open(vscode.Uri.joinPath(top, "nodg", "style.hcl"));
  await vscode.languages.setTextDocumentLanguage(helloStyle, "ini");
  await barrier("packs/c4/spec.hcl");
  assert.notEqual(otherLanguage, "dg");
  assert.notEqual(helloLanguage, "dg", "a folder without dg.hcl is not a project");

  await vscode.workspace.getConfiguration("dg", top).update("projectFolders", ["nodg"], vscode.ConfigurationTarget.WorkspaceFolder);
  await waitFor("the language dg for a folder dg.projectFolders lists", () => helloDoc.languageId === "dg");
  await waitFor("completion from the server of the listed folder", async () => (await completes(hello, "parent = node.")).includes("backend"));
  assert.equal(helloStyle.languageId, "ini", "a file another language was given keeps it");
  assert.equal(otherDoc.languageId, otherLanguage, "a file of no dg project keeps its language");
  assert.equal(vscode.languages.getDiagnostics(other).length, 0, "a file of no dg project gets no dg diagnostics");

  await vscode.workspace.getConfiguration("dg", top).update("projectFolders", undefined, vscode.ConfigurationTarget.WorkspaceFolder);
  await waitFor("the language a file had before its folder was listed", () => helloDoc.languageId === helloLanguage);

  // A project inside another is a project of its own: its files go to its server only.
  const nested = vscode.Uri.joinPath(folder, "nested", "model.hcl");
  const nestedDoc = await open(nested);
  await waitFor("the language dg for a file of a nested project", () => nestedDoc.languageId === "dg");
  const labels = await waitFor("completion from the nested project's server", async () => {
    const ls = await completes(nested, "parent = node.");
    return ls.includes("backend") && ls;
  });
  assert.ok(!labels.includes("banking"), "the outer project's server does not complete in a nested project");

  // A language the user picks for a file of a project stays.
  const views = await barrier("views.hcl");
  await vscode.languages.setTextDocumentLanguage(views, "plaintext");
  await barrier("dg.hcl");
  assert.equal(views.languageId, "plaintext");
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
