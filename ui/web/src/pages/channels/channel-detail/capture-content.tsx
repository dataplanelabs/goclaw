import { useTranslation } from "react-i18next";

import { ErrorBoundary } from "@/components/shared/error-boundary";
import { RichContent } from "@/components/chat/rich-content";

export type CaptureKind = "empty" | "rich";

export function categorizeCapture(content: string | null | undefined): CaptureKind {
  const trimmed = content?.trim() ?? "";
  if (!trimmed) return "empty";
  return "rich";
}

// Customer / team turns are joined with single \n in the poll worker, but markdown
// collapses single newlines to spaces. Promote each \n to a hard line break (two
// spaces + \n) so each captured message renders on its own line in the chat bubble.
export function preserveLineBreaks(text: string): string {
  return text.replace(/\n/g, "  \n");
}

export interface CaptureContentProps {
  content: string;
  role: "user" | "assistant";
}

export function CaptureContent({ content, role }: CaptureContentProps) {
  const { t } = useTranslation("channels");
  const kind = categorizeCapture(content);

  if (kind === "empty") {
    return <span className="text-muted-foreground italic">{t("teamAnalytics.captureEmpty")}</span>;
  }

  return (
    <ErrorBoundary
      fallback={
        <span className="text-muted-foreground italic">{t("teamAnalytics.captureRenderFailed")}</span>
      }
    >
      <RichContent content={preserveLineBreaks(content.trim())} role={role} inlineMediaUrls />
    </ErrorBoundary>
  );
}
