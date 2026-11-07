import { describe, expect, it } from 'vitest';
import type { KindInfo, ModelInfo } from '../protocol/client';
import { paletteGroups } from './palette';

const kind = (name: string, pack?: string, category?: string, short?: string): KindInfo => ({ name, pack, category, short });

const many = (n: number, pack: string, category?: string) => Array.from({ length: n }, (_, i) => kind(`${pack}-k${i}`, pack, category, `k${i}`));

const model = (kinds: KindInfo[], packs = ['generic', 'aws', 'local']): ModelInfo => ({ packs, kinds, edge_kinds: [], tags: [], nodes: [], edges: [] });

const own = Array.from({ length: 31 }, (_, i) => kind(`own${i}`));

const small = model([
  kind('aws-lambda', 'aws', 'Compute', 'lambda'),
  kind('aws-vpc', 'aws', 'Networking', 'vpc'),
  kind('aws-bucket', 'aws', 'Compute', 'bucket'),
  kind('aws-thing', 'aws', undefined, 'thing'),
  kind('aws-edited'),
  kind('mine'),
  kind('job', 'local'),
  kind('node', 'generic'),
]);

const cloud = model(
  [
    kind('gcp-cloud-run', 'gcp', 'Compute', 'cloud-run'),
    kind('gcp-key-vault', 'gcp', 'Security', 'key-vault'),
    kind('gcp-pub-sub', 'gcp', 'Messaging', 'pub-sub'),
    kind('gcp-ci', 'gcp', 'Developer Tools', 'ci'),
    kind('aws-dynamodb', 'aws', 'Database', 'dynamodb'),
    kind('aws-iam', 'aws', 'Security & Identity', 'iam'),
    kind('aws-cdn', 'aws', 'Networking & Content Delivery', 'cdn'),
    kind('aws-s3_bucket', 'aws', 'Storage', 's3_bucket'),
  ],
  ['gcp', 'aws'],
);

// shape writes groups as "pack: category[a b] ...", with * on a closed category.
const shape = (m: ModelInfo, q = '', opened = new Map<string, boolean>()) =>
  paletteGroups(m, q, opened).map((p) => `${p.title}: ${p.categories.map((c) => `${c.open ? '' : '*'}${c.title}[${c.kinds.map((k) => k.name).join(' ')}]`).join(' ')}`);

describe('paletteGroups', () => {
  it.each<[string, ModelInfo, string, Map<string, boolean>, string[]]>([
    ['packs in load order, the project last, categories sorted, kinds without one first', small, '', new Map(), ['generic: [node]', 'aws: [aws-thing] Compute[aws-lambda aws-bucket] Networking[aws-vpc]', 'local: [job]', 'Project: [aws-edited mine]']],
    ['a name without its prefix', small, 'VPC', new Map(), ['aws: Networking[aws-vpc]']],
    ['a name with its prefix', small, 'aws-b', new Map(), ['aws: Compute[aws-bucket]']],
    ['a category', small, 'comp', new Map(), ['aws: Compute[aws-lambda aws-bucket]']],
    ['a space for a hyphen', cloud, 'CLOUD RUN', new Map(), ['gcp: Compute[gcp-cloud-run]']],
    ['a hyphen for a space', cloud, 'key-vault', new Map(), ['gcp: Security[gcp-key-vault]']],
    ['no hyphen for a hyphen', cloud, 'pubsub', new Map(), ['gcp: Messaging[gcp-pub-sub]']],
    ['with the prefix and a space', cloud, 'gcp cloud', new Map(), ['gcp: Compute[gcp-cloud-run]']],
    ['a category with a hyphen for a space', cloud, 'developer-tools', new Map(), ['gcp: Developer Tools[gcp-ci]']],
    ['the start of a prefixed name past its first word', cloud, 'gcp-p', new Map(), ['gcp: Messaging[gcp-pub-sub]']],
    ['nothing across the end of a prefix', cloud, 'sd', new Map(), []],
    ['nothing across the end of another prefix', cloud, 'gk', new Map(), []],
    ['a space for an ampersand in a category', cloud, 'security identity', new Map(), ['aws: Security & Identity[aws-iam]']],
    ['a category up to its ampersand and past it', cloud, 'networking content', new Map(), ['aws: Networking & Content Delivery[aws-cdn]']],
    ['a space for an underscore', cloud, 's3 bucket', new Map(), ['aws: Storage[aws-s3_bucket]']],
    ['a filter of only separators is no filter', model(many(31, 'aws', 'A')), ' & - _ ', new Map(), [`aws: *A[${many(31, 'aws').map((k) => k.name).join(' ')}]`]],
    ['categories sorted without regard to case', model([kind('c', 'aws', 'DevOps'), kind('b', 'aws', 'Developer Tools'), kind('e', 'aws', 'database'), kind('d', 'aws', 'Database')]), '', new Map(), ['aws: Database[d] database[e] Developer Tools[b] DevOps[c]']],
    ['no match', small, 'nothing', new Map(), []],
    ['a category the user closed', small, '', new Map([['aws/Compute', false]]), ['generic: [node]', 'aws: [aws-thing] *Compute[aws-lambda aws-bucket] Networking[aws-vpc]', 'local: [job]', 'Project: [aws-edited mine]']],
    ['a filter opens what the user closed', small, 'lambda', new Map([['aws/Compute', false]]), ['aws: Compute[aws-lambda]']],
    ['the 30-kind rule leaves the project alone', model(own), '', new Map(), [`Project: [${own.map((k) => k.name).join(' ')}]`]],
  ])('%s', (_, m, q, opened, want) => {
    expect(shape(m, q, opened)).toEqual(want);
  });

  it('starts a pack of more than 30 kinds with its categories closed', () => {
    const open = paletteGroups(model(many(30, 'aws', 'A')), '', new Map());
    expect(open[0].categories.map((c) => c.open)).toEqual([true]);
    const big = paletteGroups(model([...many(30, 'aws', 'A'), kind('aws-z', 'aws', undefined, 'z')]), '', new Map());
    expect(big[0].categories.map((c) => [c.title, c.open])).toEqual([
      ['No category', false],
      ['A', false],
    ]);
  });

  it('keeps a big pack open once the user opened it, and opens the groups with a match', () => {
    const m = model([...many(20, 'aws', 'A'), ...many(20, 'aws', 'B').map((k) => ({ ...k, name: k.name + 'b', short: k.short + 'b' }))]);
    expect(paletteGroups(m, '', new Map([['aws/B', true]]))[0].categories.map((c) => c.open)).toEqual([false, true]);
    expect(paletteGroups(m, 'k3b', new Map())[0].categories.map((c) => [c.title, c.open])).toEqual([['B', true]]);
  });
});
