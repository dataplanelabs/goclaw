import { Component, Fragment, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import i18n from "@/i18n";
import { toast } from "@/stores/use-toast-store";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  retryKey: number;
}

// Stale-chunk patterns raised by browsers when Vite-hashed assets from a prior
// deploy are referenced by a still-mounted page after the deploy rotated them.
const chunkErrorPatterns = [
  /Failed to fetch dynamically imported module/i,
  /Loading chunk \d+ failed/i,
  /ChunkLoadError/i,
  /Importing a module script failed/i,
];
const reloadGuardKey = "errorBoundary:chunkReloadTried";

function isStaleChunkError(error: Error): boolean {
  const msg = `${error?.name ?? ""} ${error?.message ?? ""}`;
  return chunkErrorPatterns.some((re) => re.test(msg));
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, retryKey: 0 };

  static getDerivedStateFromError(): Partial<State> {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[ErrorBoundary]", error, info.componentStack);

    // Stale-chunk recovery: one-shot reload via sessionStorage flag so a
    // genuinely-broken chunk (404 on both old and new deploy) can't loop.
    if (isStaleChunkError(error)) {
      try {
        if (sessionStorage.getItem(reloadGuardKey) !== "1") {
          sessionStorage.setItem(reloadGuardKey, "1");
          window.location.reload();
          return;
        }
      } catch {
        // sessionStorage unavailable (private mode, etc.) — fall through.
      }
    }

    const msg = (error.message || "Unknown error").split("\n")[0]?.slice(0, 200);
    toast.error(error.name || "Render error", msg);
  }

  private handleRetry = () => {
    this.setState((prev) => ({ hasError: false, retryKey: prev.retryKey + 1 }));
  };

  render() {
    if (!this.state.hasError) {
      return <Fragment key={this.state.retryKey}>{this.props.children}</Fragment>;
    }

    if (this.props.fallback) return this.props.fallback;

    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border bg-card p-8 text-center">
        <AlertTriangle className="h-8 w-8 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">{i18n.t("common:errorBoundary")}</p>
        <Button variant="outline" size="sm" className="gap-1.5" onClick={this.handleRetry}>
          <RefreshCw className="h-3.5 w-3.5" />
          {i18n.t("common:retry")}
        </Button>
      </div>
    );
  }
}
