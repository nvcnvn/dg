# Packs

A pack is a spec and a style shipped together: kinds of nodes and edges, and how they are drawn,
with the pictures they use. The built-in `generic` pack is always loaded. This page covers
installed packs, which `dg pack` copies into a project from a git repository, and how to write
one. A local pack, a directory the project lists in `packs`, is
[part of the project](reference/language.md#the-project-file).

- [Using a pack](#using-a-pack)
- [Writing a pack](#writing-a-pack)
- [Listing a pack in the index](#listing-a-pack-in-the-index)

## Using a pack

Give `dg pack add` a git repository, with a version if you want one other than the latest:

```sh
dg pack add git.example.com/acme/network
dg pack add git.example.com/acme/network@v1.2.0
```

`add` writes a `pack` block in `dg.hcl`, the pack's files in `packs/network/` and an entry in
`dg.lock`, then runs `dg check`. It prints where the pack's license is, or warns that it has none.
Commit all three: the copy is part of the project, so a clone needs no install step, and
`dg check`, `dg serve` and `dg export` never use the network.

A version is a tag or a full commit hash, by default the highest `vX.Y.Z` tag. A source is
`<host>/<path>`, fetched over HTTPS, or an `https://`, `ssh://` or `file://` URL, with `//<dir>` at
its end for a pack that is not at the repository's root:

```sh
dg pack add ssh://git@git.example.com/acme/diagrams.git//packs/network
```

A source with no `/` names a pack listed in [the index](#listing-a-pack-in-the-index): for such a
pack `dg pack add <name>` is enough, and `dg pack search <text>` looks through the list. The index
is not published yet, so until it is, install a pack by its repository.

Every name a pack defines takes its local name as a prefix: the project writes
`kind = "network-router"` on a node, `kind.network-router` in a selector and
`style "network-router"` in its own `style.hcl`, which overrides the properties it sets. The
palette shows the kind as `router`, under the pack's name and category. `dg vocabulary` lists what
a project can use, with the prefixes.

The local name is the source's last segment. Two packs that yield one name are an error, which
`--as` resolves, and it also installs two versions of a pack side by side:

```sh
dg pack add git.example.com/acme/network@v2.0.0 --as network2
```

Later on:

```sh
dg pack update network          # to the highest vX.Y.Z tag
dg pack update network v1.3.0   # or to this version
dg pack remove network
dg pack install                 # writes each pack in dg.lock again
```

An update changes `dg.hcl`, `dg.lock` and the pack's files, and no model, view or layout file: the
source and version never appear in a name. `install` restores packs a project chose not to commit
and repairs edited ones, and refuses a pack whose files do not match the hash in `dg.lock`.
`dg check` warns when a pack's files no longer match, so an edit there does not go unnoticed; to
change a pack's look, override its styles from the project instead.

[The command line reference](reference/cli.md#dg-pack) gives the exact rules, and
[the language reference](reference/language.md#installed-packs) what `dg check` reports.

## Writing a pack

A pack is a git repository, or a directory of one, with a `spec.hcl`, a `style.hcl` or both.
[`examples/installed-pack`](https://github.com/nvcnvn/dg/blob/main/examples/installed-pack/packs/homelab/spec.hcl)
holds a complete one:

```text
spec.hcl          kinds and edge kinds
style.hcl         shapes and styles
icons/*.svg       pictures, at any depth
README.md         what the pack is for
LICENSE           the terms of the pack
NOTICE            the terms of anything in it that has its own
```

Write the short names; the prefix is the project's choice, not the pack's:

```hcl
# spec.hcl
kind "vpc" {
  extends  = "group"
  category = "Networking"
}

edge_kind "peering" {}
```

```hcl
# style.hcl
style "vpc" {
  icon = file("icons/vpc.svg")
}

edge_style "peering" {
  stroke_dash = [4, 3]
}
```

`category` groups kinds in the palette, in sorted order without regard to case, kinds without one
first. A pack of more than 30 kinds starts with its categories closed, so give a large pack
categories. A category is not inherited.

The prefix is applied for you, and a pack reaches nothing outside its names: it styles its own
kinds and tags, extends its own kinds and the generic pack's, and `dg pack add` copies only its
HCL, its license and notice files, its README and its pictures.
[The language reference](reference/language.md#installed-packs) gives the rules on names, on what
a pack may reach and on what a copy holds.

**Pictures.** Each is an SVG file, well-formed with an `svg` root element in the SVG namespace. dg
always draws a picture as an `<image>` from a `data:` URI, in the editor and in exports, so a
browser neither runs a script in it nor loads anything it names: a picture must hold everything it
draws.

**License.** Put a `LICENSE` at the pack's root: `dg pack add` points the user to it and warns when
there is none. When the pack holds art under other terms, such as a vendor's icons, say so in
`NOTICE`, and publish the art only if those terms allow it.

**Publishing.** Push the repository and tag a release `v1.0.0`. Tags are how users pick a version
and what they get by default; a branch is refused, since it moves. To try the pack first, tag it
and install it into a project from its directory:

```sh
git -C /path/to/my-pack tag v0.1.0
dg pack add file:///path/to/my-pack
dg check
```

Tag again and run `dg pack update my-pack` to try a change, and `dg pack remove my-pack` before you
commit the project, since `dg.hcl` holds that path.

## Listing a pack in the index

The index only finds packs: `dg pack add <name>` and `dg pack search` read `index.json` in the
repository `github.com/dg-packs/index`, and any git URL works without it. That repository is not
published yet. Once it is, a pack joins by pull request, adding an entry to that file:

```json
{"name": "network", "source": "git.example.com/acme/network", "description": "Routers, switches and firewalls", "license": "Apache-2.0"}
```

`name` is what users type, and should be the source's last segment, which becomes the pack's local
name. `dg.hcl` holds the `source` it resolves to. `dg pack search` shows the `description`, and
`license` says what terms the pack is under. The index holds no versions: users get the source's
highest `vX.Y.Z` tag.
