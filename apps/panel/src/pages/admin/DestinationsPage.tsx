import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { ProviderLogo } from "@/components/provider-logo";
import { dangerButtonClass, fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Destination, type DestinationList, type StorageProvider, type StorageProviderList } from "@/lib/harbor-api";
import { Cloud } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

const emptyForm = {
  provider: "cloudflare_r2",
  name: "",
  account_id: "",
  access_key_id: "",
  secret_access_key: "",
  bucket: "",
  jurisdiction: "",
  region: "",
  endpoint: "",
  key_prefix: "harbor",
  allow_over_free: false,
};

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

export default function DestinationsPage() {
  const [items, setItems] = useState<Destination[] | null>(null);
  const [providers, setProviders] = useState<StorageProvider[]>([]);
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const selected = providers.find((item) => item.id === form.provider) || providers[0];
  const groups = useMemo(() => {
    const order: string[] = [];
    for (const provider of providers) {
      if (!order.includes(provider.group)) order.push(provider.group);
    }
    return order.map((group) => ({ group, items: providers.filter((item) => item.group === group) }));
  }, [providers]);

  async function load() {
    const [destinations, catalog] = await Promise.all([
      api<DestinationList>("/api/v1/destinations"),
      api<StorageProviderList>("/api/v1/destination-providers"),
    ]);
    setItems(destinations.items);
    setProviders(catalog.items);
  }

  useEffect(() => {
    load().catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load destinations"));
  }, []);

  async function createDestination(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await api("/api/v1/destinations", {
        method: "POST",
        body: JSON.stringify(form),
      });
      const name = selected?.name || "Storage";
      setForm({ ...emptyForm, provider: form.provider });
      setMessage(`${name} is connected. Choose it on a project to copy backups there.`);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not add this destination");
    } finally {
      setSaving(false);
    }
  }

  if (!items && !error) return <AdminLoading message="Loading destinations…" />;

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader title="Destinations" />
      {error && <p className="text-sm text-error-500">{error}</p>}
      {message && <p className="text-sm text-success-600">{message}</p>}

      <section className="admin-card">
        <div className="admin-card-body space-y-4">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Add storage</h2>
          <p className="max-w-2xl text-sm leading-relaxed text-gray-500">
            Backups always stay on this machine. A destination keeps a second copy in object storage.
            Harbor checks the bucket before saving. Consumer drives such as Google Drive, Dropbox, and OneDrive are not listed, because they need a different sign-in than a bucket key.
          </p>
          <form onSubmit={createDestination} className="grid gap-4 md:grid-cols-2">
            <div className="space-y-4 md:col-span-2">
              <span className={labelClass}>Service</span>
              {groups.map((group) => (
                <div key={group.group}>
                  <p className="mb-2 text-sm text-gray-500">{group.group}</p>
                  <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4">
                    {group.items.map((provider) => {
                      const active = form.provider === provider.id;
                      return (
                        <button
                          key={provider.id}
                          type="button"
                          onClick={() => setForm({ ...form, provider: provider.id, allow_over_free: false })}
                          className={`flex items-center gap-2 rounded-lg border px-2.5 py-2 text-left ${active ? "border-brand-500 bg-brand-50 ring-2 ring-brand-500/20 dark:bg-brand-500/10" : "border-gray-200 bg-white hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-white/5"}`}
                        >
                          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-white">
                            <ProviderLogo id={provider.id} />
                          </span>
                          <span className="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100">{provider.name}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
            {selected && (
              <div className="space-y-2 md:col-span-2">
                <p className="text-sm text-gray-600 dark:text-gray-300">{selected.summary}</p>
                {selected.free_note && <p className="text-sm text-gray-500">{selected.free_note}</p>}
                <ol className="list-decimal space-y-1 pl-5 text-sm text-gray-600 dark:text-gray-300">
                  {selected.steps.map((step) => <li key={step}>{step}</li>)}
                </ol>
              </div>
            )}
            <label className="block">
              <span className={labelClass}>Name</span>
              <input className={fieldClass} required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder={selected?.name || "Backups"} />
            </label>
            <label className="block">
              <span className={labelClass}>Bucket</span>
              <input className={fieldClass} required value={form.bucket} onChange={(e) => setForm({ ...form, bucket: e.target.value })} placeholder="harbor-backups" />
            </label>
            {selected?.account_id && (
              <label className="block md:col-span-2">
                <span className={labelClass}>Account ID</span>
                <input className={fieldClass} required value={form.account_id} onChange={(e) => setForm({ ...form, account_id: e.target.value })} placeholder="Account ID, or the full endpoint from the dashboard" />
              </label>
            )}
            {selected?.region && (
              <label className="block">
                <span className={labelClass}>Region</span>
                <input className={fieldClass} required value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder={selected.region_placeholder} />
              </label>
            )}
            {selected?.endpoint && (
              <label className={`block ${selected.region ? "" : "md:col-span-2"}`}>
                <span className={labelClass}>Endpoint</span>
                <input className={fieldClass} required value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder={selected.endpoint_placeholder} />
              </label>
            )}
            {selected?.jurisdiction && (
              <label className="block">
                <span className={labelClass}>Jurisdiction</span>
                <select className={fieldClass} value={form.jurisdiction} onChange={(e) => setForm({ ...form, jurisdiction: e.target.value })}>
                  <option value="">Default</option>
                  <option value="eu">European Union</option>
                  <option value="fedramp">FedRAMP</option>
                </select>
              </label>
            )}
            <label className="block">
              <span className={labelClass}>Access key</span>
              <input className={fieldClass} required autoComplete="off" value={form.access_key_id} onChange={(e) => setForm({ ...form, access_key_id: e.target.value })} />
            </label>
            <label className="block">
              <span className={labelClass}>Secret key</span>
              <input className={fieldClass} required type="password" autoComplete="new-password" value={form.secret_access_key} onChange={(e) => setForm({ ...form, secret_access_key: e.target.value })} />
            </label>
            <label className="block">
              <span className={labelClass}>Key prefix</span>
              <input className={fieldClass} value={form.key_prefix} onChange={(e) => setForm({ ...form, key_prefix: e.target.value })} />
            </label>
            {selected && selected.free_bytes > 0 && (
              <label className="flex items-center gap-2 self-end text-sm text-gray-600 dark:text-gray-300">
                <input type="checkbox" checked={form.allow_over_free} onChange={(e) => setForm({ ...form, allow_over_free: e.target.checked })} />
                Keep copying after the free {formatCap(selected.free_bytes)}
              </label>
            )}
            <div className="md:col-span-2">
              <button className={primaryButtonClass} disabled={saving || !selected} type="submit">{saving ? "Checking bucket…" : "Add destination"}</button>
            </div>
          </form>
        </div>
      </section>

      {items && items.length === 0 ? (
        <div className="admin-card">
          <AdminEmptyState icon={Cloud} title="No destinations yet" description="Until you add one, every backup stays on this machine." />
        </div>
      ) : (
        <div className="grid gap-4">
          {items?.map((destination) => {
            const limit = destination.free_storage_bytes || 0;
            const accountUsed = destination.account_used_bytes || 0;
            const width = limit > 0 ? Math.min(100, (Math.min(limit, accountUsed) / limit) * 100) : 0;
            const where = [destination.bucket, destination.region, destination.endpoint, destination.jurisdiction && `${destination.jurisdiction} jurisdiction`].filter(Boolean).join(" · ");
            return (
              <article key={destination.id} className="admin-card">
                <div className="admin-card-body space-y-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="flex items-center gap-3">
                      <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-white">
                        <ProviderLogo id={destination.provider} className="h-8 w-8" />
                      </span>
                      <div>
                        <h2 className="text-base font-semibold text-gray-900 dark:text-white">{destination.name}</h2>
                        <p className="text-sm text-gray-500">{destination.provider_name || destination.provider} · {where}</p>
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <button
                        type="button"
                        className={quietButtonClass}
                        onClick={() => {
                          setError(null);
                          setMessage(null);
                          void api(`/api/v1/destinations/${destination.id}/test`, { method: "POST" })
                            .then(() => setMessage(`${destination.name} can see the bucket.`))
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not reach this storage"));
                        }}
                      >
                        Test
                      </button>
                      <button
                        type="button"
                        className={dangerButtonClass}
                        onClick={() => {
                          if (!confirm(`Remove ${destination.name}? Backups stay on this machine. Files already in the bucket stay there.`)) return;
                          setError(null);
                          void api(`/api/v1/destinations/${destination.id}`, { method: "DELETE" })
                            .then(load)
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not remove destination"));
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                  {limit > 0 && !destination.allow_over_free && (
                    <>
                      <p className="text-sm text-gray-500">{formatBytes(accountUsed)} of {formatCap(limit)} stored by Harbor on this account</p>
                      <div className="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800">
                        <div className="h-full bg-brand-500" style={{ width: `${width}%` }} />
                      </div>
                    </>
                  )}
                  {destination.allow_over_free && (
                    <p className="text-sm text-gray-500">Harbor will keep copying after the free allowance. {formatBytes(accountUsed)} is stored so far.</p>
                  )}
                  <p className="text-xs text-gray-500">Access key {destination.access_key_id} · prefix {destination.key_prefix}</p>
                </div>
              </article>
            );
          })}
        </div>
      )}
    </div>
  );
}
