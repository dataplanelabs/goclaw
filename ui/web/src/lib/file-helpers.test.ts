import { describe, it, expect } from "vitest";
import { buildTree, mergeSubtree, type TreeNode } from "./file-helpers";

const dir = (path: string, name: string): TreeNode => ({
  name,
  path,
  isDir: true,
  size: 0,
  hasChildren: true,
  children: [],
});

describe("mergeSubtree", () => {
  // Regression: loadSubtree returns children with FULL (tenant-relative) paths.
  // mergeSubtree must graft only parentPath's direct descendants, not the whole
  // rebuilt ancestor chain (which produced a duplicated path under the node).
  it("does not re-nest the ancestor chain under the target folder", () => {
    const parent = "zalo-shtp/group_x/generated/2026-05-29";
    const tree: TreeNode[] = [{ ...dir(parent, "2026-05-29"), loading: true }];
    const children = [
      { path: `${parent}/poster-a.png`, name: "poster-a.png", isDir: false, size: 1 },
      { path: `${parent}/poster-b.png`, name: "poster-b.png", isDir: false, size: 2 },
    ];

    const node = mergeSubtree(tree, parent, children)[0];
    expect(node).toBeDefined();
    if (!node) return;

    expect(node.path).toBe(parent);
    expect(node.loading).toBe(false);
    expect(node.children.map((c) => c.name).sort()).toEqual(["poster-a.png", "poster-b.png"]);
    // No phantom intermediate dir (e.g. a re-nested date/group chain).
    expect(node.children.every((c) => !c.isDir)).toBe(true);
    expect(node.children.find((c) => c.path === parent)).toBeUndefined();
  });

  it("handles basename-only children via fallback", () => {
    const node = mergeSubtree([dir("folder", "folder")], "folder", [
      { path: "a.txt", name: "a.txt", isDir: false, size: 1 },
    ])[0];
    expect(node).toBeDefined();
    if (!node) return;
    expect(node.children.map((c) => c.name)).toEqual(["a.txt"]);
  });
});

describe("buildTree", () => {
  it("nests by path and de-dupes", () => {
    const roots = buildTree([
      { path: "d", name: "d", isDir: true, size: 0, hasChildren: true },
      { path: "d/x.png", name: "x.png", isDir: false, size: 1 },
    ]);
    expect(roots).toHaveLength(1);
    const root = roots[0];
    expect(root).toBeDefined();
    if (!root) return;
    expect(root.path).toBe("d");
    expect(root.children.map((c) => c.name)).toEqual(["x.png"]);
  });

  it("sorts labeled/named folders before unresolved bare-id folders, by visible name", () => {
    const roots = buildTree([
      { path: "900000000000000002", name: "900000000000000002", isDir: true }, // bare id, no label
      { path: "zalo-annhien", name: "zalo-annhien", isDir: true }, // named dir
      { path: "100000000000000001", name: "100000000000000001", isDir: true, label: "Writer One" }, // labeled
      { path: "100000000000000003", name: "100000000000000003", isDir: true, label: "Aaa Sales" }, // labeled
    ].map((f) => ({ ...f, size: 0 })));
    // Labeled (by label) + named first (alpha), bare id last.
    expect(roots.map((r) => r.label || r.name)).toEqual([
      "Aaa Sales",
      "Writer One",
      "zalo-annhien",
      "900000000000000002",
    ]);
  });

  it("carries modTime through to the node", () => {
    const roots = buildTree([
      { path: "f.md", name: "f.md", isDir: false, size: 3, modTime: "2026-05-29T18:00:00Z" },
    ]);
    expect(roots[0]?.modTime).toBe("2026-05-29T18:00:00Z");
  });
});
