# Editor

`dg serve` opens the editor in the browser, and the VS Code extension's "dg: Open diagram" opens
the same editor in a panel. Every change it makes is an [operation](operations.md) written to the
project's files at once; there is nothing to save. Layout gestures write the view's layout file;
model edits write the model files and `views.hcl`.

## The window

- The **toolbar** is one row of groups: the view switcher, breadcrumbs and New view…; Undo and
  Redo; Add node…, Connect with the edge kind it draws, and Decoration…; Unpin and Arrange (align
  and distribute); zoom; the palette and inspector buttons; Export; and `?`. Rest the pointer on a
  button, or move the focus to it, to read what it does and its shortcut, or what enables it.
  Groups that do not fit go into the menu at the end of the row.
- The **palette** on the left lists the kinds of the loaded packs with a picture of each, under
  their pack's name, with a filter.
- The **inspector** on the right edits the selection.
- **Breadcrumbs** in a view with `of` lead back to the views that show its container.

The palette and inspector start hidden in a window under 1,000 pixels wide, such as a VS Code
panel. The editor follows the system's light or dark theme, and in VS Code the editor's theme; the
drawing itself keeps the project's colors on white.

A view opens fitted to the window. A view with no nodes says how to add one. In a view with nothing
pinned, a message says the arrangement is automatic and that dragging a node keeps it; in a view
with some nodes pinned, unpinned nodes are faded.

When the project has warnings or errors, a bar under the toolbar counts them; click it to list
them, and in VS Code click one to open its file. While there are errors the editor shows the last
good drawing, read-only, until the files are fixed; with no good drawing to show, it lists the
errors in the middle of the canvas.

## Selecting and viewing

- Click a node or edge to select it; Shift-click adds or removes a node; drag a box on empty
  canvas to select the nodes inside it (Shift adds them); Ctrl/⌘+A selects every node.
- Pan with the scroll bars, the middle mouse button, or Space and drag. Zoom with Ctrl/⌘ and the
  wheel, `+` (or `=`), `-` and `0` (actual size), or the toolbar. Shift+1 fits the view and
  Shift+2 zooms to the selection; the zoom level in the toolbar opens a menu of both and of 50%,
  100% and 200%.
- Switch views with the view switcher, a node's drill-down badge, or the breadcrumbs.

## Arranging

Each gesture is sent when it is dropped, as one undo step.

- **Move:** drag nodes. A node stays inside its container, which grows to hold it unless its size
  is pinned. In a view with nothing pinned, the first drag pins every node where it is drawn. A
  multi-node selection moves as one.
- **Resize:** drag a corner handle of a selected node.
- **Reorder:** drag a child of a stack or grid container to another place in it.
- **Ports:** drag a port handle of a selected node along its border to pin the port there.
- **Edges:** select an edge, then drag one of its ends to another side of its node, or its middle
  label along it. Double-click the edge to add a via point, which can then be dragged;
  double-click a via point to remove it. "Reset route" in its context menu removes its hints.
- **Unpin** (U), **Align** and **Distribute** act on the selection, from the toolbar or the
  context menu. Unpinned nodes return to automatic placement.
- **Move into a container:** hold Alt (Option) while dragging to drop a node into the container
  under the pointer, or out onto the canvas. This changes the model's `parent`; a toast offers to
  undo it.
- Escape cancels a drag; with nothing to cancel, it clears the selection.

## Editing the model

- **Create:** drag a kind from the palette onto the canvas or into a container, or click it (or
  press Enter on it) to create the node in the middle of the view. Type its label at once: the label also gives the
  node its ID.
- **Labels:** double-click a node or an edge's label, or press F2 or Enter, to edit it in place. On
  a node with a [label in this view](views.md#labels-in-a-view), that edits the view's label.
  Enter commits, Shift+Enter breaks the line, Escape cancels.
- **Connect:** press C or the Connect button, then drag from a node, port or table row to another
  to draw an edge of the kind chosen beside the button. Escape leaves connect mode.
- **Inspector:** edit the selected node's label, its label in this view (empty: the model's), kind, tags, the properties its kind declares,
  its ports, and the rows of a table or class; an edge's kind, labels and tags; a decoration's
  text. Each field is written when it changes.
- **Copy and paste:** Ctrl/⌘+C copies the selected nodes with the nodes inside them and the edges
  among them; Ctrl/⌘+V pastes them at the pointer with new IDs.
- **Delete:** Delete or Backspace deletes the selection from the model (a decoration from the
  view).
- **Add node…** shows a node of the model that the view leaves out. **New view…** creates a view
  from a title, an ID, selectors and overlays. **Decoration…** adds a text, rectangle, ellipse,
  band, image, arrow or freehand line.

A node or edge defined in a [`read_only`](language.md#the-project-file) file shows a lock: it can
be moved and resized, but not edited.

## The context menu

Right-click, Shift+F10 or the menu key opens it; the arrow keys move through it. An item that does
not apply is left out; one refused for a reason, such as a read-only file, is shown disabled with
the reason.

| On | Items |
|---|---|
| nodes | Edit label, Copy, Paste, Hide from view, Delete from model, Delete with contents, Rename ID…, Add port…, Open view …, Unpin, Align, Distribute |
| an edge | Edit label, Rename ID…, Delete edge, Reset route |
| a decoration | Promote to node…, Attach to node… or Detach from node, Delete decoration |
| the canvas | Paste here, Add node…, Add text here |

"Hide from view" changes the view's selectors, not the model.

## Undo

Undo and Redo, in the toolbar or Ctrl/⌘+Z and Ctrl/⌘+Shift+Z (or Ctrl+Y), take back the
session's changes, from whichever browser window made them. An undo keeps edits made in a text
editor since, in the same files; it fails, and drops the change from the history, when such an
edit touched the same lines.

## Exporting

Export in the toolbar saves the current view as SVG, the same file `dg export --format svg` writes,
or as PNG, one pixel per unit on white.

## Keyboard shortcuts

The canvas's keys apply while the focus is on the canvas, not in the toolbar, panels, menus or
dialogs; click the canvas to bring it back.

| Key | Does |
|---|---|
| Ctrl/⌘+Z, Ctrl/⌘+Shift+Z, Ctrl+Y | undo, redo |
| Ctrl/⌘+A | select every node |
| Ctrl/⌘+C, Ctrl/⌘+V | copy, paste |
| Delete, Backspace | delete the selection |
| F2, Enter | edit the selected label |
| C | connect mode |
| U | unpin the selection |
| `+` or `=`, `-`, `0` | zoom in, zoom out, actual size |
| Shift+1, Shift+2 | fit, zoom to selection |
| Space+drag | pan |
| Shift+F10, menu key | context menu |
| `?` | this list of shortcuts |
| Escape | cancel a drag, leave connect mode, close a menu, or else clear the selection |
