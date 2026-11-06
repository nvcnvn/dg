// exampleNames are the examples a playground project can hold, the one the page opens first; a
// test keeps the list equal to examples/.
export const exampleNames = ['hello', 'class', 'er'];

export type Project = Record<string, string>;

// The page shows these two files. A project can also hold views.hcl and more layouts, which hiding
// a node or making a view adds.
export const MODEL_FILE = 'model.hcl';
export const STYLE_FILE = 'style.hcl';

const projectFile = (path: string) => path === MODEL_FILE || path === STYLE_FILE || path === 'views.hcl' || /^views\/[^/]+\.layout\.yaml$/.test(path);

// asProject returns value when it is a project of at least one file, and throws otherwise.
export function asProject(value: unknown): Project {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error('not a project');
  const files = Object.entries(value);
  if (!files.length || files.some(([path, text]) => !projectFile(path) || typeof text !== 'string')) throw new Error('not a project');
  return value as Project;
}

// sameProject reports whether a and b hold the same files.
export function sameProject(a: Project, b: Project): boolean {
  const sorted = (p: Project) => JSON.stringify(Object.entries(p).sort(([x], [y]) => (x < y ? -1 : 1)));
  return sorted(a) === sorted(b);
}

const sources = import.meta.glob('../../../examples/{hello,class,er}/{model.hcl,style.hcl,views/main.layout.yaml}', { query: '?raw', import: 'default', eager: true }) as Record<string, string>;

// example returns the files of the example called name, by their paths in the project.
export function example(name: string): Project {
  const prefix = `../../../examples/${name}/`;
  return Object.fromEntries(
    Object.entries(sources)
      .filter(([path]) => path.startsWith(prefix))
      .map(([path, text]) => [path.slice(prefix.length), text]),
  );
}
