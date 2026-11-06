import '@fontsource/inter/400.css';
import '@fontsource/inter/700.css';
import '../theme.css';
import '../app.css';
import './playground.css';
import { ask, h } from '../canvas/dialog';
import { startEditor } from '../canvas/editor';
import type { Diagnostic } from '../protocol/client';
import { newElk, render } from '../renderer';
import { showMenu } from '../canvas/menu';
import { asProject, example, exampleNames, MODEL_FILE, sameProject, STYLE_FILE, type Project } from './examples';
import { LONG, shareFragment, sharedFiles } from './share';
import { textEditor, type TextEditor } from './text';
import { transport, type PageMessage } from './transport';

declare const Go: new () => { importObject: WebAssembly.Imports; run(instance: WebAssembly.Instance): Promise<void> };

declare global {
  var dgToGo: ((json: string) => void) | undefined;
  var dgToPage: (json: string) => void;
}

const SAVE_MS = 300;
const STORE = 'dg-playground';

const message = document.getElementById('message')!;
const say = (text: string) => {
  message.textContent = text;
};

const OPENING = `${STORE}-opening`;

function stored(): Project | undefined {
  try {
    const json = localStorage.getItem(STORE);
    return json === null ? undefined : asProject(JSON.parse(json));
  } catch {
    return undefined;
  }
}

// opening marks a project being opened, so that one that stops Go is not opened again.
function opening(on: boolean): boolean {
  try {
    const was = localStorage.getItem(OPENING) !== null;
    if (on) localStorage.setItem(OPENING, '1');
    else localStorage.removeItem(OPENING);
    return was;
  } catch {
    return false;
  }
}

// The files as Go last showed them; the text editors hold what the visitor typed since.
let project: Project = {};
let started = false;
let stopped = false;
let saving = new Map<string, number>();

const editors = new Map<string, TextEditor>();
const current = (): Project => {
  const files = { ...project };
  for (const [path, editor] of editors) {
    const text = editor.text();
    if (text !== '' || path in files) files[path] = text;
  }
  return files;
};

function store() {
  if (!started) return;
  try {
    localStorage.setItem(STORE, JSON.stringify(current()));
  } catch {
    say('This browser could not keep your project, so a reload would lose it. Export can download it.');
  }
}

const save = (path: string) => {
  clearTimeout(saving.get(path));
  saving.delete(path);
  t.saveFile(path, editors.get(path)!.text());
};
const flush = () => [...saving.keys()].forEach(save);

const typed = (path: string) => () => {
  clearTimeout(saving.get(path));
  saving.set(
    path,
    window.setTimeout(() => {
      save(path);
      store();
    }, SAVE_MS),
  );
};

const stop = () => {
  stopped = true;
  say('The playground stopped. Reload the page to go on.');
};

function fromGo(m: PageMessage) {
  switch (m.type) {
    case 'ready':
      void start();
      break;
    case 'files':
      project = m.files ?? {};
      for (const [path, editor] of editors) if (!saving.has(path)) editor.show(project[path] ?? '');
      store();
      break;
    case 'loaded':
      for (const editor of editors.values()) editor.forget();
      started = true;
      opening(false);
      store();
      document.body.classList.remove('loading');
      break;
    case 'diagnostics':
      for (const editor of editors.values()) editor.diagnostics(m.diagnostics);
      break;
  }
}

// Go runs inside dgToGo and can answer before it returns; the editor reads a request's seq from the
// send it is still in, so like a WebSocket this may only answer later.
const t = transport((json) => queueMicrotask(() => stopped || globalThis.dgToGo?.(json)), flush, fromGo);

// An exception here would be a panic in Go, which sent the message.
globalThis.dgToPage = (json) => {
  try {
    t.receive(json);
  } catch (err) {
    console.error(err);
    say(`Something went wrong: ${err}`);
  }
};

// Go's stack is the browser's; overflowing it leaves Go dead with nothing said.
window.addEventListener('error', (ev) => {
  if (ev.error instanceof RangeError || ev.error instanceof WebAssembly.RuntimeError) stop();
});

editors.set(MODEL_FILE, textEditor(document.getElementById('model-text')!, MODEL_FILE, 'Model', typed(MODEL_FILE)));
editors.set(STYLE_FILE, textEditor(document.getElementById('style-text')!, STYLE_FILE, 'Style', typed(STYLE_FILE)));

function load(files: Project) {
  saving.forEach((id) => clearTimeout(id));
  saving = new Map();
  started = false;
  opening(true);
  t.load(files);
}

// linked returns the project of the page's share link, and takes the link out of the address.
async function linked(): Promise<Project | undefined> {
  if (!location.hash) return undefined;
  try {
    return await sharedFiles(location.hash);
  } catch {
    say('This share link is damaged, so it was not opened.');
    return undefined;
  } finally {
    history.replaceState(null, '', location.pathname + location.search);
  }
}

// mayReplace reports whether files can go: they are an example as it came, or the visitor says so.
const mayReplace = (files: Project, by: string) => exampleNames.some((name) => sameProject(files, example(name))) || confirm(`Replace your diagram with ${by}?`);

// start opens the project of a share link, else the last one in this browser, else hello.
async function start() {
  const shared = await linked();
  const failed = opening(false);
  if (failed) say('Your last project stopped the playground, so hello opened instead.');
  const last = failed ? undefined : stored();
  const hello = example(exampleNames[0]);
  const keep = shared && last && !sameProject(shared, last) && !mayReplace(last, 'the one in this link');
  load(!shared || keep ? (last ?? hello) : shared);
}

window.addEventListener('hashchange', () => {
  void linked().then((shared) => {
    if (shared && (sameProject(current(), shared) || mayReplace(current(), 'the one in this link'))) load(shared);
  });
});

const examples = document.getElementById('examples')!;
const menu = document.getElementById('tool-menu')!;
examples.addEventListener('click', () => {
  const r = examples.getBoundingClientRect();
  const open = (name: string) => () => {
    if (!mayReplace(current(), 'this example')) return;
    say('');
    load(example(name));
  };
  showMenu(menu, exampleNames.map((name) => ({ label: name, on: true, run: open(name) })), r.left, r.bottom, examples);
});

document.getElementById('share')!.addEventListener('click', () => {
  (async () => {
    const link = location.href.split('#')[0] + (await shareFragment(current()));
    const copied = await navigator.clipboard?.writeText(link).then(
      () => true,
      () => false,
    );
    const help = [copied ? 'The link is copied.' : 'Copy this link.', link.length > LONG ? `At ${link.length} characters, some chat apps will cut it.` : ''].join(' ');
    await ask('Share this diagram', [{ name: 'link', label: 'Link', value: link, type: 'textarea', help }], 'Close');
  })().catch((err) => say(`Sharing failed: ${err}`));
});

const download = async () => {
  try {
    const data = Uint8Array.from(atob(await t.zip()), (c) => c.charCodeAt(0));
    const a = h('a', { href: URL.createObjectURL(new Blob([data], { type: 'application/zip' })), download: 'diagram.zip' });
    a.click();
    URL.revokeObjectURL(a.href);
  } catch (err) {
    say(`Download failed: ${err}`);
  }
};

window.addEventListener('pagehide', store);

const elk = newElk();
startEditor((view) => render(view, elk), {
  name: 'the playground',
  connect: t.connect,
  openDiagnostic: (d: Diagnostic) => {
    if (d.file && d.line) editors.get(d.file)?.reveal(d.line, d.column ?? 1);
  },
  saves: () => [{ label: 'Download project (.zip)', on: true, run: () => void download() }],
});

async function run() {
  await new Promise((resolve, reject) => {
    document.head.append(h('script', { src: 'wasm_exec.js', onload: resolve, onerror: () => reject(new Error('wasm_exec.js did not load')) }));
  });
  const go = new Go();
  const { instance } = await WebAssembly.instantiateStreaming(fetch('dg.wasm'), go.importObject);
  await go.run(instance);
  stop();
}

run().catch((err) => say(`The playground could not start: ${err}`));
