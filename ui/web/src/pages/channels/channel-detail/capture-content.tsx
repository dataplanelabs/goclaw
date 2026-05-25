import { useTranslation } from "react-i18next";

import { ErrorBoundary } from "@/components/shared/error-boundary";
import { RichContent } from "@/components/chat/rich-content";

export type CaptureKind = "empty" | "rich";

export function categorizeCapture(content: string | null | undefined): CaptureKind {
  const trimmed = content?.trim() ?? "";
  if (!trimmed) return "empty";
  return "rich";
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
      <RichContent content={content.trim()} role={role} inlineMediaUrls />
    </ErrorBoundary>
  );
}
