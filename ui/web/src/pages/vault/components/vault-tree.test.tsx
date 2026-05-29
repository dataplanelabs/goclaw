import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { VaultTree } from "./vault-tree";
import type { TreeNode } from "@/lib/file-helpers";
import type { VaultTreeEntry } from "../hooks/use-vault-tree";

function folder(overrides: Partial<TreeNode>): TreeNode {
  return { name: "x", path: "x", isDir: true, size: 0, hasChildren: false, children: [], ...overrides };
}

function renderTree(tree: TreeNode[]) {
  return render(
    <VaultTree
      tree={tree}
      meta={new Map<string, VaultTreeEntry>()}
      loading={false}
      activePath={null}
      onSelect={vi.fn()}
      onLoadMore={vi.fn()}
      treeVersion={0}
    />,
  );
}

describe("VaultTree label rendering", () => {
  afterEach(cleanup);

  it("renders the display label as primary with the dimmed id beside it", () => {
    renderTree([
      folder({
        name: "group_zalo-shtp_900000000000000001",
        path: "zalo-shtp/group_zalo-shtp_900000000000000001",
        label: "Đơn Hàng",
        kind: "group",
      }),
    ]);
    expect(screen.getByText("Đơn Hàng")).toBeTruthy();
    expect(screen.getByText("·group")).toBeTruthy();
    // Raw id still present (dimmed), carrying a title attr for hover.
    expect(screen.getByText("group_zalo-shtp_900000000000000001")).toBeTruthy();
  });

  it("falls back to the raw folder name when no label is resolved", () => {
    renderTree([folder({ name: "teams", path: "teams" })]);
    expect(screen.getByText("teams")).toBeTruthy();
    expect(screen.queryByText(/·/)).toBeNull();
  });

  it("shows the label without a kind pill when kind is undefined", () => {
    renderTree([folder({ name: "100000000000000001", path: "zalo-shtp/100000000000000001", label: "Writer One" })]);
    expect(screen.getByText("Writer One")).toBeTruthy();
    expect(screen.getByText("100000000000000001")).toBeTruthy();
    expect(screen.queryByText(/·/)).toBeNull();
  });
});
