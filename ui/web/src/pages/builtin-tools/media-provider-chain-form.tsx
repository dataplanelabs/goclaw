import { useState, useCallback, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  arrayMove,
  sortableKeyboardCoordinates,
} from "@dnd-kit/sortable";
import { Plus, Loader2 } from "lucide-react";
import { uniqueId } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { useProviders } from "@/pages/providers/hooks/use-providers";
import { SortableProviderCard } from "./media-sortable-provider-card";
import type { ProviderEntry } from "./media-provider-chain-helpers";
import { formatToolTitle, parseInitialEntries } from "./media-provider-chain-helpers";

interface MediaProviderChainFormProps {
  toolName: string;
  initialSettings: Record<string, unknown>;
  onSave: (settings: Record<string, unknown>) => Promise<void>;
  onCancel: () => void;
}

/** Dialog form for configuring an ordered provider chain for a media tool. */
export function MediaProviderChainForm({
  toolName,
  initialSettings,
  onSave,
  onCancel,
}: MediaProviderChainFormProps) {
  const { t } = useTranslation("tools");
  const { providers } = useProviders();
  const enabledProviders = useMemo(() => providers.filter((p) => p.enabled), [providers]);
  const portalRef = useRef<HTMLDivElement | null>(null);

  const [entries, setEntries] = useState<ProviderEntry[]>(() =>
    parseInitialEntries(initialSettings, providers),
  );
  const [saving, setSaving] = useState(false);
  const [dailyLimit, setDailyLimit] = useState<string>(() => {
    const v = initialSettings.daily_limit;
    return typeof v === "number" && v > 0 ? String(v) : "";
  });
  const [dailyScope, setDailyScope] = useState<string>(() => {
    const s = initialSettings.daily_limit_scope;
    return typeof s === "string" && s ? s : "channel";
  });

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event;
    if (over && active.id !== over.id) {
      setEntries((prev) => {
        const oldIndex = prev.findIndex((e) => e.id === active.id);
        const newIndex = prev.findIndex((e) => e.id === over.id);
        return arrayMove(prev, oldIndex, newIndex);
      });
    }
  }, []);

  const handleUpdate = useCallback((id: string, patch: Partial<ProviderEntry>) => {
    setEntries((prev) => prev.map((e) => (e.id === id ? { ...e, ...patch } : e)));
  }, []);

  const handleRemove = useCallback((id: string) => {
    setEntries((prev) => prev.filter((e) => e.id !== id));
  }, []);

  const handleAdd = () => {
    setEntries((prev) => [
      ...prev,
      {
        id: uniqueId(),
        provider_id: "",
        provider: "",
        model: "",
        enabled: true,
        timeout: 120,
        max_retries: 2,
        params: {},
      },
    ]);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const serialized = entries.map(({ id: _id, ...rest }) => rest);
      // Preserve settings keys this form doesn't manage (don't clobber daily_limit etc.).
      const next: Record<string, unknown> = { ...initialSettings, providers: serialized };
      const limit = Number.parseInt(dailyLimit, 10);
      if (Number.isFinite(limit) && limit > 0) {
        next.daily_limit = limit;
        next.daily_limit_scope = dailyScope;
      } else {
        delete next.daily_limit;
        delete next.daily_limit_scope;
      }
      await onSave(next);
    } catch {
      // toast shown by hook
    } finally {
      setSaving(false);
    }
  };

  return (
    <div ref={portalRef} className="relative">
      <DialogHeader>
        <DialogTitle>{formatToolTitle(toolName)} {t("builtin.mediaChain.providerChainSuffix")}</DialogTitle>
        <DialogDescription>
          {t("builtin.mediaChain.description")}
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2 max-h-[60vh] overflow-y-auto pr-1 my-4">
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={entries.map((e) => e.id)} strategy={verticalListSortingStrategy}>
            {entries.map((entry, index) => (
              <SortableProviderCard
                key={entry.id}
                entry={entry}
                index={index}
                toolName={toolName}
                enabledProviders={enabledProviders}
                onUpdate={handleUpdate}
                onRemove={handleRemove}
                portalRef={portalRef}
              />
            ))}
          </SortableContext>
        </DndContext>

        {entries.length === 0 && (
          <p className="text-sm text-muted-foreground text-center py-6">
            {t("builtin.mediaChain.noProviders")}
          </p>
        )}

        <Button type="button" variant="outline" size="sm" className="w-full" onClick={handleAdd}>
          <Plus className="size-3.5 mr-1.5" />
          {t("builtin.mediaChain.addProvider")}
        </Button>
      </div>

      <div className="rounded-md border p-3 mb-4 space-y-2">
        <div className="flex items-center gap-3 flex-wrap">
          <label className="text-sm font-medium whitespace-nowrap">{t("builtin.mediaChain.dailyLimit")}</label>
          <Input
            type="number"
            min={0}
            value={dailyLimit}
            onChange={(e) => setDailyLimit(e.target.value)}
            placeholder="0"
            className="w-24"
          />
          <select
            value={dailyScope}
            onChange={(e) => setDailyScope(e.target.value)}
            aria-label={t("builtin.mediaChain.scope")}
            className="h-9 rounded-md border border-input bg-background px-2 text-sm"
          >
            <option value="channel">{t("builtin.mediaChain.scopeChannel")}</option>
            <option value="thread">{t("builtin.mediaChain.scopeThread")}</option>
            <option value="agent">{t("builtin.mediaChain.scopeAgent")}</option>
            <option value="tool">{t("builtin.mediaChain.scopeTool")}</option>
          </select>
        </div>
        <p className="text-xs text-muted-foreground">{t("builtin.mediaChain.dailyLimitHelp")}</p>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          {t("builtin.mediaChain.cancel")}
        </Button>
        <Button onClick={handleSave} disabled={saving}>
          {saving && <Loader2 className="h-4 w-4 animate-spin" />}
          {saving ? t("builtin.mediaChain.saving") : t("builtin.mediaChain.save")}
        </Button>
      </DialogFooter>
    </div>
  );
}
