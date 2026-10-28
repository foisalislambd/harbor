import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { ProviderLogo } from "@/components/provider-logo";
import { dangerButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Destination, type DestinationList, type StorageProvider, type StorageProviderList } from "@/lib/harbor-api";
import { Cloud } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useLocation } from "react-router-dom";

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

export default function DestinationsPage() {
  const location = useLocation();
  const [items, setItems] = useState<Destination[] | null>(null);
  const [providers, setProviders] = useState<StorageProvider[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

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
    const note = (location.state as { message?: string } | null)?.message;
    if (note) setMessage(note);
    load().catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load destinations"));
  }, [location.state]);

  if (!items && !error) return <AdminLoading message="Loading destinations…" />;

  return (
    <div className="admin-content space-y-3">
      <AdminPageHeader compact title="Destinations" />
      {error && <p className="text-sm text-error-500">{error}</p>}
      {message && <p className="text-sm text-success-600">{message}</p>}

      <section className="admin-card">
        <div className="admin-card-body space-y-4">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Add storage</h2>
          <p className="max-w-2xl text-sm leading-relaxed text-gray-500">
            Backups always stay on this machine. A destination keeps a second copy in object storage.
            Harbor checks the bucket before saving. Consumer drives such as Google Drive, Dropbox, and OneDrive are not listed, because they need a different sign-in than a bucket key.
          </p>
          <div className="space-y-4">
            {groups.map((group) => (
              <div key={group.group}>
                <p className="mb-2 text-sm text-gray-500">{group.group}</p>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4">
                  {group.items.map((provider) => (
                    <Link
                      key={provider.id}
                      to={`/destinations/new/${provider.id}`}
                      className="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-left hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-white/5"
                    >
                      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-white">
                        <ProviderLogo id={provider.id} />
                      </span>
                      <span className="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100">{provider.name}</span>
                    </Link>
                  ))}
                </div>
              </div>
            ))}
          </div>
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
