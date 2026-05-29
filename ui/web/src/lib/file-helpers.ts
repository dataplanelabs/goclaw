export interface TreeNode {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  hasChildren?: boolean;
  loading?: boolean;
  protected?: boolean;
  label?: string; // human display name (e.g. contact/group name) for id folders
  kind?: string; // peer kind: "direct" | "group"
  children: TreeNode[];
}

export const CODE_EXTENSIONS = new Set([
  "ts", "tsx", "js", "jsx", "py", "go", "json", "yaml", "yml", "toml",
  "sh", "bash", "css", "html", "sql", "rs", "rb", "java", "kt", "swift",
  "c", "cpp", "h", "xml", "graphql", "proto", "lua", "zig", "env",
]);

export const IMAGE_EXTENSIONS = new Set([
  "jpg", "jpeg", "png", "gif", "webp", "svg", "ico", "bmp",
]);

const TEXT_EXTENSIONS = new Set([
  ...CODE_EXTENSIONS, ...["md", "csv", "txt", "log", "cfg", "ini", "conf"],
]);

export function extOf(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot >= 0 ? name.slice(dot + 1).toLowerCase() : "";
}

export function isImageFile(name: string): boolean {
  return IMAGE_EXTENSIONS.has(extOf(name));
}

export function isTextFile(name: string): boolean {
  return TEXT_EXTENSIONS.has(extOf(name));
}

export function langFor(ext: string): string {
  const map: Record<string, string> = {
    ts: "typescript", tsx: "tsx", js: "javascript", jsx: "jsx",
    py: "python", go: "go", rs: "rust", rb: "ruby", sh: "bash", bash: "bash",
    yml: "yaml", yaml: "yaml", json: "json", toml: "toml",
    css: "css", html: "html", sql: "sql", xml: "xml",
    java: "java", kt: "kotlin", swift: "swift",
    c: "c", cpp: "cpp", h: "c", graphql: "graphql",
    proto: "protobuf", lua: "lua", zig: "zig",
  };
  return map[ext] ?? ext;
}

/** Returns badge variant based on file size. */
export function sizeBadgeVariant(bytes: number): "outline" | "info" | "warning" | "destructive" {
  if (bytes < 100 * 1024) return "outline";
  if (bytes < 1024 * 1024) return "info";
  if (bytes < 10 * 1024 * 1024) return "warning";
  return "destructive";
}

interface TreeInput {
  path: string;
  name: string;
  isDir: boolean;
  size: number;
  hasChildren?: boolean;
  protected?: boolean;
  label?: string;
  kind?: string;
}

export function buildTree(files: TreeInput[]): TreeNode[] {
  const root: TreeNode[] = [];
  const dirMap = new Map<string, TreeNode>();

  const attachNode = (node: TreeNode) => {
    const parentPath = node.path.includes("/")
      ? node.path.slice(0, node.path.lastIndexOf("/"))
      : "";

    if (parentPath) {
      const parent = ensureDir(parentPath);
      if (!parent.children.some((child) => child.path === node.path)) {
        parent.children.push(node);
      }
      return;
    }

    if (!root.some((child) => child.path === node.path)) {
      root.push(node);
    }
  };

  const ensureDir = (path: string): TreeNode => {
    const existing = dirMap.get(path);
    if (existing) return existing;

    const node: TreeNode = {
      name: path.split("/").pop() ?? path,
      path,
      isDir: true,
      size: 0,
      children: [],
    };
    dirMap.set(path, node);
    attachNode(node);
    return node;
  };

  const sorted = [...files].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return a.path.localeCompare(b.path);
  });

  for (const f of sorted) {
    const node: TreeNode = {
      name: f.name,
      path: f.path,
      isDir: f.isDir,
      size: f.size,
      hasChildren: f.hasChildren,
      protected: f.protected,
      label: f.label,
      kind: f.kind,
      children: [],
    };

    if (f.isDir) {
      dirMap.set(f.path, node);
    }

    attachNode(node);
  }

  const sortChildren = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    for (const n of nodes) {
      if (n.children.length > 0) sortChildren(n.children);
    }
  };
  sortChildren(root);
  return root;
}

function findNodeByPath(nodes: TreeNode[], path: string): TreeNode | undefined {
  for (const node of nodes) {
    if (node.path === path) return node;
    const found = findNodeByPath(node.children, path);
    if (found) return found;
  }
  return undefined;
}

/** Merges a loaded subtree into an existing tree, replacing the target folder's children. */
export function mergeSubtree(tree: TreeNode[], parentPath: string, newChildren: TreeInput[]): TreeNode[] {
  // newChildren carry full paths, so buildTree reconstructs the whole ancestor
  // chain — take only parentPath's own descendants, else they'd be re-nested
  // under a duplicate path chain inside the target node.
  const built = buildTree(newChildren);
  const directChildren = findNodeByPath(built, parentPath)?.children ?? built;

  const merge = (nodes: TreeNode[]): TreeNode[] =>
    nodes.map((node) => {
      if (node.path === parentPath && node.isDir) {
        return { ...node, children: directChildren, hasChildren: false, loading: false };
      }
      if (node.children.length > 0) {
        return { ...node, children: merge(node.children) };
      }
      return node;
    });

  return merge(tree);
}

/** Marks a node as loading in the tree (immutable update). */
export function setNodeLoading(tree: TreeNode[], path: string, loading: boolean): TreeNode[] {
  return tree.map((node) => {
    if (node.path === path) return { ...node, loading };
    if (node.children.length > 0) return { ...node, children: setNodeLoading(node.children, path, loading) };
    return node;
  });
}

export function stripFrontmatter(content: string): string {
  if (!content.startsWith("---")) return content;
  const end = content.indexOf("---", 3);
  if (end < 0) return content;
  return content.slice(end + 3).replace(/^\n+/, "");
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Convert a local file path to a /v1/files/ URL for serving.
 *  Extracts basename so the backend fallback search can find generated files.
 *  Server signs URLs with ?ft= at delivery time — no client-side token needed. */
export function toFileUrl(path: string): string {
  if (path.startsWith("/v1/files/") || path.includes("/v1/files/")) {
    return path;
  }
  const basename = path.split("/").pop() ?? path;
  return `/v1/files/${basename}`;
}

/** Append ?download=true to a URL for Content-Disposition: attachment. */
export function toDownloadUrl(url: string): string {
  return url + (url.includes("?") ? "&" : "?") + "download=true";
}

/** Determine MediaItem kind from MIME type */
export function mediaKindFromMime(mime: string): "image" | "video" | "audio" | "document" | "code" {
  if (mime.startsWith("image/")) return "image";
  if (mime.startsWith("video/")) return "video";
  if (mime.startsWith("audio/")) return "audio";
  const ext = mime.split("/").pop() ?? "";
  if (["javascript", "typescript", "python", "json", "xml", "html", "css"].some((t) => ext.includes(t))) return "code";
  return "document";
}
