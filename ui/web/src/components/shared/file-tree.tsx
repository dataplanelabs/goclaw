import { useState, useEffect, useCallback, useMemo } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { DndContext, DragOverlay } from "@dnd-kit/core";
import { Folder, FolderOpen, ChevronRight, Loader2, Trash2, Copy } from "lucide-react";
import { formatSize, type TreeNode } from "@/lib/file-helpers";
import { formatRelativeTime } from "@/lib/format";
import { toast } from "@/stores/use-toast-store";
import { useTreeDnd } from "@/hooks/use-tree-dnd";
import { DragPreview } from "@/components/shared/drag-preview";
import { FileIcon } from "./file-tree-file-icon";
import { DraggableItem, DroppableFolder, RootDropZone } from "./file-tree-dnd-wrappers";

export function TreeItem({
  node,
  depth,
  activePath,
  onSelect,
  onDelete,
  onLoadMore,
  dndEnabled,
  autoExpandPath,
  showSize,
  baseDir,
}: {
  node: TreeNode;
  depth: number;
  activePath: string | null;
  onSelect: (path: string) => void;
  onDelete?: (path: string, isDir: boolean) => void;
  onLoadMore?: (path: string) => void;
  dndEnabled: boolean;
  autoExpandPath: string | null;
  showSize?: boolean;
  baseDir?: string;
}) {
  const { t } = useTranslation("common");
  const [expanded, setExpanded] = useState(depth === 0);
  const isActive = activePath === node.path;

  // Full absolute server path for the row, for copy-to-debug. baseDir is the
  // server-side root (e.g. /app/workspace/tenants/shtp); node.path is relative.
  const serverPath = baseDir ? `${baseDir.replace(/\/$/, "")}/${node.path}` : node.path;
  const copyBtn = baseDir && (
    <button
      type="button"
      className="ml-1 shrink-0 opacity-0 group-hover/tree-item:opacity-100 text-muted-foreground hover:text-foreground transition-opacity cursor-pointer p-0.5"
      title={serverPath}
      onClick={(e) => {
        e.stopPropagation();
        const w = navigator.clipboard?.writeText(serverPath);
        if (w) {
          w.then(
            () => toast.success(t("pathCopied"), serverPath),
            () => toast.error(t("pathCopyFailed"), serverPath),
          );
        } else {
          toast.error(t("pathCopyFailed"), serverPath);
        }
      }}
    >
      <Copy className="h-3.5 w-3.5" />
    </button>
  );

  // Name display: when a folder resolves to a human label (contact/group name),
  // show the name as primary with the raw id dimmed beside it + a kind pill.
  const nameNode = node.label ? (
    <span className="flex min-w-0 items-baseline gap-1.5">
      <span className="truncate font-medium">{node.label}</span>
      {node.kind && (
        <span className="shrink-0 text-2xs text-muted-foreground">·{node.kind}</span>
      )}
      <span className="truncate text-2xs text-muted-foreground/70 tabular-nums" title={node.name}>
        {node.name}
      </span>
    </span>
  ) : (
    <span className="truncate">{node.name}</span>
  );

  // Auto-expand folder when hovered during drag for 800ms.
  useEffect(() => {
    if (autoExpandPath === node.path && node.isDir && !expanded) {
      setExpanded(true);
      if (node.hasChildren && node.children.length === 0 && !node.loading) {
        onLoadMore?.(node.path);
      }
    }
  }, [autoExpandPath, node.path, node.isDir, expanded, node.hasChildren, node.children.length, node.loading, onLoadMore]);

  const handleToggle = useCallback(() => {
    const willExpand = !expanded;
    setExpanded(willExpand);
    if (willExpand && node.isDir && node.hasChildren && node.children.length === 0 && !node.loading) {
      onLoadMore?.(node.path);
    }
  }, [expanded, node.isDir, node.hasChildren, node.children.length, node.loading, node.path, onLoadMore]);

  const deleteBtn = onDelete && !node.protected && (
    <button
      type="button"
      className="ml-auto shrink-0 opacity-0 group-hover/tree-item:opacity-100 text-destructive hover:text-destructive/80 transition-opacity cursor-pointer p-0.5"
      title={node.isDir ? t("deleteFolder") : t("deleteFile")}
      onClick={(e) => { e.stopPropagation(); onDelete(node.path, node.isDir); }}
    >
      <Trash2 className="h-3.5 w-3.5" />
    </button>
  );

  const sizeLabel = showSize && !node.isDir && (node.size > 0 || node.modTime) && (
    <span className="ml-auto shrink-0 text-2xs text-muted-foreground tabular-nums">
      {node.size > 0 && formatSize(node.size)}
      {node.size > 0 && node.modTime && " · "}
      {node.modTime && formatRelativeTime(node.modTime)}
    </span>
  );

  if (node.isDir) {
    const folderContent = (isDropTargetActive: boolean) => (
      <>
        <div
          className={`group/tree-item flex w-full items-center gap-1 rounded px-2 py-1 text-left text-sm cursor-pointer ${
            isDropTargetActive ? "bg-primary/10 ring-1 ring-primary" : "hover:bg-accent"
          }`}
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
          onClick={handleToggle}
        >
          <ChevronRight
            className={`h-3 w-3 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`}
          />
          {expanded ? (
            <FolderOpen className="h-4 w-4 shrink-0 text-yellow-600" />
          ) : (
            <Folder className="h-4 w-4 shrink-0 text-yellow-600" />
          )}
          {nameNode}
          {node.loading && <Loader2 className="h-3 w-3 shrink-0 animate-spin text-muted-foreground ml-1" />}
          {sizeLabel}
          {copyBtn}
          {deleteBtn}
        </div>
        {expanded && node.children.map((child) => (
          <TreeItem
            key={child.path}
            node={child}
            depth={depth + 1}
            activePath={activePath}
            onSelect={onSelect}
            onDelete={onDelete}
            onLoadMore={onLoadMore}
            dndEnabled={dndEnabled}

            autoExpandPath={autoExpandPath}
            showSize={showSize}
            baseDir={baseDir}
          />
        ))}
        {expanded && node.hasChildren && node.children.length === 0 && !node.loading && (
          <div
            className="flex items-center gap-1 text-xs text-muted-foreground cursor-pointer hover:text-foreground"
            style={{ paddingLeft: `${(depth + 1) * 16 + 20}px` }}
            onClick={() => onLoadMore?.(node.path)}
          >
            <Loader2 className="h-3 w-3" />
            <span>{t("loadMore")}</span>
          </div>
        )}
      </>
    );

    if (dndEnabled) {
      return (
        <DraggableItem id={node.path} enabled>
          <DroppableFolder id={node.path} enabled>
            {({ isDropTarget: active }) => folderContent(active)}
          </DroppableFolder>
        </DraggableItem>
      );
    }

    return <div>{folderContent(false)}</div>;
  }

  // File node
  const fileContent = (
    <div
      className={`group/tree-item flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm cursor-pointer ${
        isActive ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
      }`}
      style={{ paddingLeft: `${depth * 16 + 20}px` }}
      onClick={() => onSelect(node.path)}
    >
      <FileIcon name={node.name} />
      {nameNode}
      {sizeLabel}
      {copyBtn}
      {deleteBtn}
    </div>
  );

  if (dndEnabled) {
    return (
      <DraggableItem id={node.path} enabled>
        {fileContent}
      </DraggableItem>
    );
  }

  return fileContent;
}

/** Find a node by path in the tree. */
function findNode(tree: TreeNode[], path: string): TreeNode | undefined {
  for (const node of tree) {
    if (node.path === path) return node;
    if (node.children.length > 0) {
      const found = findNode(node.children, path);
      if (found) return found;
    }
  }
  return undefined;
}

export function FileTreePanel({
  tree,
  filesLoading,
  activePath,
  onSelect,
  onDelete,
  onLoadMore,
  onMove,
  showSize,
  baseDir,
}: {
  tree: TreeNode[];
  filesLoading: boolean;
  activePath: string | null;
  onSelect: (path: string) => void;
  onDelete?: (path: string, isDir: boolean) => void;
  onLoadMore?: (path: string) => void;
  onMove?: (fromPath: string, toFolder: string) => void;
  showSize?: boolean;
  baseDir?: string;
}) {
  const { t } = useTranslation("common");
  const { sensors, activeId, autoExpandPath, handlers } = useTreeDnd(onMove);
  const dndEnabled = !!onMove;

  // Find the active node for DragOverlay preview.
  const activeNode = useMemo(
    () => (activeId ? findNode(tree, activeId) : undefined),
    [activeId, tree],
  );

  if (filesLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (tree.length === 0) {
    return <p className="px-3 py-4 text-sm text-muted-foreground">{t("noFiles")}</p>;
  }

  const treeContent = (
    <div className="flex-1 min-h-0">
      {/* Root-level drop target for moving to root */}
      {dndEnabled ? (
        <RootDropZone>
          {tree.map((node) => (
            <TreeItem
              key={node.path} node={node} depth={0} activePath={activePath}
              onSelect={onSelect} onDelete={onDelete} onLoadMore={onLoadMore}
              dndEnabled={dndEnabled} autoExpandPath={autoExpandPath}
              showSize={showSize} baseDir={baseDir}
            />
          ))}
        </RootDropZone>
      ) : (
        tree.map((node) => (
          <TreeItem
            key={node.path} node={node} depth={0} activePath={activePath}
            onSelect={onSelect} onDelete={onDelete} onLoadMore={onLoadMore}
            dndEnabled={false} autoExpandPath={null}
            showSize={showSize} baseDir={baseDir}
          />
        ))
      )}
    </div>
  );

  if (!dndEnabled) return treeContent;

  return (
    <DndContext sensors={sensors} {...handlers}>
      {treeContent}
      {/* Portal to document.body so DragOverlay isn't offset by Radix Dialog's CSS transform. */}
      {createPortal(
        <DragOverlay dropAnimation={null}>
          {activeNode ? (
            <DragPreview name={activeNode.name} isDir={activeNode.isDir} />
          ) : null}
        </DragOverlay>,
        document.body,
      )}
    </DndContext>
  );
}

