# Changelog

## 0.1.3

First release on Open VSX.

- The extension's `dg` comes first on the `PATH` of the editor's terminals, so you, the panel and
  an agent started there share one version
- A project written for a newer dg says so, with dg's version and advice to update
- An unknown attribute lists what is expected, and errors and `dg op --help` show nested fields

## 0.1.2

First release on the Marketplace.

- Language server for dg projects: diagnostics, completion, hover, Go to Definition, Find All
  References, Rename Symbol and Format Document
- Diagram panel: arrange, create, connect and edit in the same editor as `dg serve`, with undo
- Bundled `dg` binary for Linux, macOS and Windows on x64 and arm64
