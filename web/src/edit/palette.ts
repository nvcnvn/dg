import { cmp } from '../geometry/geometry';
import type { KindInfo, ModelInfo } from '../protocol/client';

// OPEN_KINDS is the most kinds a pack has with its categories open at first (FR-20).
const OPEN_KINDS = 30;

// PaletteCategory is a group of kinds of a pack; one with no title shows them under the pack's name.
export interface PaletteCategory {
  key: string;
  title: string;
  kinds: KindInfo[];
  open: boolean;
}

// PalettePack is a pack in the palette, or the project's own kinds, with its categories.
export interface PalettePack {
  title: string;
  categories: PaletteCategory[];
}

// bare drops what a filter ignores in a name: case and all but letters and digits.
const bare = (s: string) => s.toLowerCase().replace(/[^\p{L}\p{N}]/gu, '');

// paletteGroups returns what the palette shows for query, or for "" all of it: the packs in load
// order, the project's own last, each by category in sorted order without regard to case, kinds without one first. A
// category is open if query has a match, or else as opened says, by key, or else unless its pack
// is large; the project is not a pack, so its own kinds never are. Packs and categories without a match are left out.
export function paletteGroups(model: ModelInfo, query: string, opened: ReadonlyMap<string, boolean>): PalettePack[] {
  const q = bare(query);
  // Only the start of a prefixed name matches: anywhere in it, bare joins the prefix to the name,
  // and "sd" would find aws-dynamodb.
  const matches = (k: KindInfo) => bare(k.short ?? k.name).includes(q) || bare(k.name).startsWith(q) || bare(k.category ?? '').includes(q);
  return [...(model.packs ?? []), ''].flatMap((pack) => {
    const all = model.kinds.filter((k) => (k.pack ?? '') === pack);
    const small = !pack || all.length <= OPEN_KINDS;
    const categories = [...new Set(all.map((k) => k.category ?? ''))].sort((a, b) => cmp(a.toLowerCase(), b.toLowerCase()) || cmp(a, b)).flatMap((category) => {
      const kinds = all.filter((k) => (k.category ?? '') === category && matches(k));
      const key = `${pack}/${category}`;
      return kinds.length ? [{ key, title: category || (small ? '' : 'No category'), kinds, open: !!q || (opened.get(key) ?? small) }] : [];
    });
    return categories.length ? [{ title: pack || 'Project', categories }] : [];
  });
}
