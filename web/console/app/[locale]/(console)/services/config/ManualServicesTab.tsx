"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Pencil, Plus, Trash2, Wrench } from "lucide-react";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { EmptyState } from "../../../../components/ui/EmptyState";
import { useWebServices, type WebService } from "../../../../hooks/useWebServices";
import { useStatus } from "../../../../contexts/StatusContext";
import type { Asset } from "../../../../console/models";

const SERVICE_CATEGORIES = [
  "Media",
  "Downloads",
  "Gaming",
  "Networking",
  "Monitoring",
  "Home Automation",
  "Management",
  "Development",
  "Storage",
  "Databases",
  "Security",
  "Productivity",
  "Other",
] as const;

function isValidServiceURL(url: string): boolean {
  try {
    const parsed = new URL(url);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.hostname.length > 0
    );
  } catch {
    return false;
  }
}

type FormState = {
  name: string;
  url: string;
  category: string;
  hostAssetId: string;
  iconKey: string;
  tags: string;
};

const EMPTY_FORM: FormState = {
  name: "",
  url: "",
  category: "",
  hostAssetId: "",
  iconKey: "",
  tags: "",
};

type ManualServicesTabProps = {
  prefilledAssetId?: string;
};

export default function ManualServicesTab({ prefilledAssetId }: ManualServicesTabProps) {
  const t = useTranslations("services");
  const { status } = useStatus();
  const {
    services,
    loading,
    refresh,
    createManualService,
    updateManualService,
    deleteManualService,
  } = useWebServices({ includeHidden: true });

  const assets: Asset[] = useMemo(() => status?.assets ?? [], [status?.assets]);

  const assetNameMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const asset of assets) {
      map.set(asset.id, asset.name);
    }
    return map;
  }, [assets]);

  const manualServices = useMemo(
    () =>
      services.filter(
        (s) => s.source === "manual" || s.metadata?.manual === "true"
      ),
    [services]
  );

  // Form state
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [formErrors, setFormErrors] = useState<Partial<Record<keyof FormState, string>>>({});
  const [saving, setSaving] = useState(false);
  const [assetSearch, setAssetSearch] = useState("");

  // Pre-fill behavior
  useEffect(() => {
    if (prefilledAssetId) {
      setShowForm(true);
      setEditingId(null);
      setForm({ ...EMPTY_FORM, hostAssetId: prefilledAssetId });
    }
  }, [prefilledAssetId]);

  const filteredAssets = useMemo(() => {
    if (!assetSearch.trim()) return assets;
    const lower = assetSearch.toLowerCase();
    return assets.filter((a) => a.name.toLowerCase().includes(lower));
  }, [assets, assetSearch]);

  const resetForm = useCallback(() => {
    setShowForm(false);
    setEditingId(null);
    setForm(EMPTY_FORM);
    setFormErrors({});
    setAssetSearch("");
  }, []);

  const openAddForm = useCallback(() => {
    setEditingId(null);
    setForm(prefilledAssetId ? { ...EMPTY_FORM, hostAssetId: prefilledAssetId } : EMPTY_FORM);
    setFormErrors({});
    setAssetSearch("");
    setShowForm(true);
  }, [prefilledAssetId]);

  const openEditForm = useCallback((service: WebService) => {
    setEditingId(service.id);
    setForm({
      name: service.name,
      url: service.url,
      category: service.category,
      hostAssetId: service.host_asset_id,
      iconKey: service.icon_key,
      tags: service.metadata?.user_tags ?? "",
    });
    setFormErrors({});
    setAssetSearch("");
    setShowForm(true);
  }, []);

  const validate = useCallback((): boolean => {
    const errors: Partial<Record<keyof FormState, string>> = {};
    if (!form.name.trim()) {
      errors.name = t("config.manual.nameRequired");
    }
    if (!form.url.trim()) {
      errors.url = t("config.manual.urlRequired");
    } else if (!isValidServiceURL(form.url.trim())) {
      errors.url = t("config.manual.urlInvalid");
    }
    if (!form.category) {
      errors.category = t("config.manual.categoryRequired");
    }
    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  }, [form, t]);

  const handleSubmit = useCallback(async () => {
    if (saving) return;
    if (!validate()) return;
    setSaving(true);
    try {
      const metadata: Record<string, string> = {};
      const trimmedTags = form.tags.trim();
      if (trimmedTags) {
        metadata.user_tags = trimmedTags;
      }

      if (editingId) {
        await updateManualService(editingId, {
          host_asset_id: form.hostAssetId,
          name: form.name.trim(),
          url: form.url.trim(),
          category: form.category,
          icon_key: form.iconKey.trim() || undefined,
          metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
        });
      } else {
        await createManualService({
          host_asset_id: form.hostAssetId,
          name: form.name.trim(),
          url: form.url.trim(),
          category: form.category,
          icon_key: form.iconKey.trim() || undefined,
          metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
        });
      }
      resetForm();
      await refresh();
    } catch {
      // Error is handled by the hook
    } finally {
      setSaving(false);
    }
  }, [saving, validate, editingId, form, createManualService, updateManualService, resetForm, refresh]);

  const handleDelete = useCallback(
    async (service: WebService) => {
      if (!window.confirm(t("config.manual.deleteConfirm"))) return;
      try {
        await deleteManualService(service.id);
        if (editingId === service.id) {
          resetForm();
        }
        await refresh();
      } catch {
        // Error is handled by the hook
      }
    },
    [deleteManualService, editingId, resetForm, refresh, t]
  );

  const updateField = useCallback(
    <K extends keyof FormState>(field: K, value: FormState[K]) => {
      setForm((prev) => ({ ...prev, [field]: value }));
      setFormErrors((prev) => {
        if (!prev[field]) return prev;
        const next = { ...prev };
        delete next[field];
        return next;
      });
    },
    []
  );

  function statusLabel(service: WebService): string {
    if (!service.host_asset_id) {
      return t("config.manual.notMonitored");
    }
    return service.status || t("config.manual.notMonitored");
  }

  function hostLabel(service: WebService): string {
    if (!service.host_asset_id) {
      return t("config.manual.standalone");
    }
    return assetNameMap.get(service.host_asset_id) ?? service.host_asset_id;
  }

  if (loading && manualServices.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-sm text-[var(--muted)]">{t("config.manual.loading")}</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Header with Add button */}
      <div className="flex items-center justify-between">
        <div />
        {!showForm && (
          <Button variant="primary" size="sm" onClick={openAddForm}>
            <Plus className="h-3.5 w-3.5" />
            {t("config.manual.addService")}
          </Button>
        )}
      </div>

      {/* Inline add/edit form */}
      {showForm && (
        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4">
          <h3 className="text-sm font-semibold text-[var(--text)] mb-3">
            {editingId
              ? t("config.manual.editService")
              : t("config.manual.addService")}
          </h3>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {/* Name */}
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-[var(--muted)]">
                {t("config.manual.form.name")}
              </label>
              <Input
                value={form.name}
                onChange={(e) => updateField("name", e.target.value)}
                placeholder={t("config.manual.form.namePlaceholder")}
                error={Boolean(formErrors.name)}
              />
              {formErrors.name && (
                <span className="text-xs text-[var(--bad)]">{formErrors.name}</span>
              )}
            </div>

            {/* URL */}
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-[var(--muted)]">
                {t("config.manual.form.url")}
              </label>
              <Input
                value={form.url}
                onChange={(e) => updateField("url", e.target.value)}
                placeholder={t("config.manual.form.urlPlaceholder")}
                error={Boolean(formErrors.url)}
              />
              {formErrors.url && (
                <span className="text-xs text-[var(--bad)]">{formErrors.url}</span>
              )}
            </div>

            {/* Category */}
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-[var(--muted)]">
                {t("config.manual.form.category")}
              </label>
              <Select
                value={form.category}
                onChange={(e) => updateField("category", e.target.value)}
                error={Boolean(formErrors.category)}
              >
                <option value="">{t("config.manual.form.selectPlaceholder")}</option>
                {SERVICE_CATEGORIES.map((cat) => (
                  <option key={cat} value={cat}>
                    {cat}
                  </option>
                ))}
              </Select>
              {formErrors.category && (
                <span className="text-xs text-[var(--bad)]">
                  {formErrors.category}
                </span>
              )}
            </div>

            {/* Host Asset */}
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-[var(--muted)]">
                {t("config.manual.form.hostAsset")}
              </label>
              <Input
                value={assetSearch}
                onChange={(e) => {
                  setAssetSearch(e.target.value);
                  if (form.hostAssetId) {
                    updateField("hostAssetId", "");
                  }
                }}
                placeholder={t("config.manual.hostPlaceholder")}
              />
              {form.hostAssetId && (
                <div className="flex items-center gap-1.5">
                  <span className="text-xs text-[var(--text)]">
                    {assetNameMap.get(form.hostAssetId) ?? form.hostAssetId}
                  </span>
                  <button
                    type="button"
                    className="text-xs text-[var(--muted)] hover:text-[var(--text)] cursor-pointer"
                    onClick={() => {
                      updateField("hostAssetId", "");
                      setAssetSearch("");
                    }}
                  >
                    x
                  </button>
                </div>
              )}
              {!form.hostAssetId && assetSearch.trim() && filteredAssets.length > 0 && (
                <div className="max-h-36 overflow-y-auto rounded border border-[var(--line)] bg-[var(--surface)]">
                  {filteredAssets.map((asset) => (
                    <button
                      key={asset.id}
                      type="button"
                      className="w-full text-left px-2 py-1.5 text-xs text-[var(--text)] hover:bg-[var(--hover)] cursor-pointer"
                      onClick={() => {
                        updateField("hostAssetId", asset.id);
                        setAssetSearch("");
                      }}
                    >
                      {asset.name}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Icon Key */}
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-[var(--muted)]">
                {t("config.manual.form.icon")}
              </label>
              <Input
                value={form.iconKey}
                onChange={(e) => updateField("iconKey", e.target.value)}
                placeholder={t("config.manual.form.iconPlaceholder")}
              />
            </div>

            {/* Tags */}
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-[var(--muted)]">
                {t("config.manual.form.tags")}
              </label>
              <Input
                value={form.tags}
                onChange={(e) => updateField("tags", e.target.value)}
                placeholder={t("config.manual.form.tagsPlaceholder")}
              />
            </div>
          </div>

          {/* Form actions */}
          <div className="flex items-center gap-2 mt-4">
            <Button
              variant="primary"
              size="sm"
              loading={saving}
              onClick={handleSubmit}
            >
              {editingId
                ? t("config.manual.editService")
                : t("config.manual.addService")}
            </Button>
            <Button variant="ghost" size="sm" onClick={resetForm} disabled={saving}>
              {t("config.manual.form.cancel")}
            </Button>
          </div>
        </div>
      )}

      {/* Table */}
      {manualServices.length === 0 ? (
        <EmptyState
          icon={Wrench}
          title={t("config.manual.addService")}
          description={t("config.manual.emptyState")}
          action={
            !showForm ? (
              <Button variant="primary" size="sm" onClick={openAddForm}>
                <Plus className="h-3.5 w-3.5" />
                {t("config.manual.addService")}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-[var(--line)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--line)] bg-[var(--surface)]">
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.manual.table.name")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.manual.table.url")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.manual.table.category")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.manual.table.host")}
                </th>
                <th className="text-left px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.manual.table.status")}
                </th>
                <th className="text-right px-3 py-2 text-xs font-semibold text-[var(--muted)]">
                  {t("config.manual.table.actions")}
                </th>
              </tr>
            </thead>
            <tbody>
              {manualServices.map((service) => (
                <tr
                  key={service.id}
                  className="border-b border-[var(--line)] last:border-b-0 hover:bg-[var(--hover)] transition-colors"
                >
                  <td className="px-3 py-2 text-[var(--text)] font-medium">
                    {service.name}
                  </td>
                  <td className="px-3 py-2 text-[var(--muted)] max-w-[200px] truncate">
                    <a
                      href={service.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="hover:text-[var(--accent)] transition-colors"
                    >
                      {service.url}
                    </a>
                  </td>
                  <td className="px-3 py-2 text-[var(--muted)]">
                    {service.category}
                  </td>
                  <td className="px-3 py-2 text-[var(--muted)]">
                    {hostLabel(service)}
                  </td>
                  <td className="px-3 py-2 text-[var(--muted)]">
                    {statusLabel(service)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    <div className="inline-flex items-center gap-1">
                      <button
                        type="button"
                        className="p-1 rounded hover:bg-[var(--hover)] text-[var(--muted)] hover:text-[var(--text)] transition-colors cursor-pointer"
                        onClick={() => openEditForm(service)}
                        title={t("config.manual.editService")}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        className="p-1 rounded hover:bg-[var(--hover)] text-[var(--muted)] hover:text-[var(--bad)] transition-colors cursor-pointer"
                        onClick={() => handleDelete(service)}
                        title={t("config.manual.table.delete")}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
