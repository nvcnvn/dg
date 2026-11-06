// exampleNames are the examples a playground project can hold, the one the page opens first; a
// test keeps the list equal to examples/.
export const exampleNames = ['hello', 'class', 'er'];

export type Project = Record<string, string>;

// PROJECT_FILES are the files a project can hold; the page shows the first two.
export const MODEL_FILE = 'model.hcl';
export const STYLE_FILE = 'style.hcl';
const PROJECT_FILES = [MODEL_FILE, STYLE_FILE, 'views/main.layout.yaml'];

// asProject returns value when it is a project, and throws otherwise.
export function asProject(value: unknown): Project {
  if (typeof value !== 'object' || value === null || Array.isArray(value) || Object.entries(value).some(([path, text]) => !PROJECT_FILES.includes(path) || typeof text !== 'string')) throw new Error('not a project');
  return value as Project;
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
