import '@fontsource/inter/400.css';
import '@fontsource/inter/700.css';
import '../theme.css';
import '../app.css';
import './playground.css';
import { ask, h } from '../canvas/dialog';
import { startEditor } from '../canvas/editor';
import type { Diagnostic } from '../protocol/client';
import { newElk, render } from '../renderer';
import { asProject, example, exampleNames, MODEL_FILE, STYLE_FILE, type Project } from './examples';
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

function stored(): Project | undefined {
  try {
    const json = localStorage.getItem(STORE);
    return json === null ? undefined : asProject(JSON.parse(json));
  } catch {
    return undefined;
  }
}

function store(files: Project) {
  try {
    localStorage.setItem(STORE, JSON.stringify(files));
  } catch {
    // A browser that keeps nothing loses the project with the page; the page works the same.
  }
}

// The files as Go last showed them; the text editors hold what the visitor typed since.
let project: Project = {};
let opened = '';
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
      store(current());
    }, SAVE_MS),
  );
};

function fromGo(m: PageMessage) {
  switch (m.type) {
    case 'ready':
      void start();
      break;
    case 'files':
      project = m.files;
      for (const [path, editor] of editors) if (!saving.has(path)) editor.show(project[path] ?? '');
      store(current());
      break;
    case 'loaded':
      opened = JSON.stringify(current());
      break;
    case 'diagnostics':
      for (const editor of editors.values()) editor.diagnostics(m.diagnostics);
      break;
  }
}

// Go runs inside dgToGo and can answer before it returns; the editor reads a request's seq from the
// send it is still in, so like a WebSocket this may only answer later.
const t = transport((json) => queueMicrotask(() => globalThis.dgToGo!(json)), flush, fromGo);
globalThis.dgToPage = t.receive;

editors.set(MODEL_FILE, textEditor(document.getElementById('model-text')!, MODEL_FILE, 'Model', typed(MODEL_FILE)));
editors.set(STYLE_FILE, textEditor(document.getElementById('style-text')!, STYLE_FILE, 'Style', typed(STYLE_FILE)));

function load(files: Project) {
  saving.forEach((id) => clearTimeout(id));
  saving = new Map();
  t.load(files);
}

// start opens the project of a share link, else the last one in this browser, else hello.
async function start() {
  let shared: Project | undefined;
  try {
    shared = await sharedFiles(location.hash);
  } catch {
    say('This share link is damaged, so it opened your last project instead.');
  }
  if (location.hash) history.replaceState(null, '', location.pathname + location.search);
  load(shared ?? stored() ?? example(exampleNames[0]));
  document.body.classList.remove('loading');
}

const examples = document.getElementById('examples') as HTMLSelectElement;
for (const name of exampleNames) examples.append(h('option', { value: name }, name));
examples.addEventListener('change', () => {
  const name = examples.value;
  examples.value = '';
  if (!name || (JSON.stringify(current()) !== opened && !confirm('Replace your diagram with this example?'))) return;
  say('');
  load(example(name));
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

window.addEventListener('pagehide', () => store(current()));

const elk = newElk();
startEditor((view) => render(view, elk), {
  connect: t.connect,
  openDiagnostic: (d: Diagnostic) => {
    if (d.file && d.line) editors.get(d.file)?.reveal(d.line, d.column ?? 1);
  },
  saves: () => [{ label: 'Download project (.zip)', on: true, run: () => void download() }],
});

async function run() {
  await new Promise((resolve, reject) => {
    document.head.append(h('script', { src: 'wasm_exec.js', onload: resolve, onerror: reject }));
  });
  const go = new Go();
  const { instance } = await WebAssembly.instantiateStreaming(fetch('dg.wasm'), go.importObject);
  await go.run(instance);
}

run().catch((err) => say(`The playground could not start: ${err}`));
