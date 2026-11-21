import { useAdminAuth } from "@/components/admin/auth/admin-auth-provider";
import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { ProviderLogo } from "@/components/provider-logo";
import { dangerButtonClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Destination, type DestinationList } from "@/lib/harbor-api";
import { Cloud } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { TelegramSignIn } from "./TelegramSignIn";

type Check = { ok: boolean; message: string };

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

function statusOf(destination: Destination, check?: Check) {
  if (check && !check.ok) return { label: "Error", className: "bg-error-50 text-error-600" };
  if (destination.provider === "telegram" && !destination.signed_in) return { label: "Needs sign-in", className: "bg-gray-100 text-gray-700 dark:bg-white/10 dark:text-gray-200" };
  return { label: "Connected", className: "bg-success-50 text-success-600" };
}

function detail(destination: Destination) {
  if (destination.provider === "telegram") {
    return [destination.account_id, destination.endpoint || "Saved Messages", destination.signed_in ? destination.account_label : ""].filter(Boolean).join(" · ");
  }
  return [destination.bucket, destination.region, destination.endpoint].filter(Boolean).join(" · ");
}

export default function DestinationsPage() {
  const { admin } = useAdminAuth();
  const isOwner = admin?.role === "owner";
  const location = useLocation();
  const [items, setItems] = useState<Destination[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const flash = (location.state as { message?: string } | null)?.message ?? null;
  const [message, setMessage] = useState<string | null>(flash);
  const [checks, setChecks] = useState<Record<string, Check>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [appliedFlash, setAppliedFlash] = useState(flash);
  if (flash !== appliedFlash) {
    setAppliedFlash(flash);
    if (flash) setMessage(flash);
  }

  async function load() {
    const destinations = await api<DestinationList>("/api/v1/destinations");
    setItems(destinations.items);
  }

  useEffect(() => {
    let cancelled = false;
    api<DestinationList>("/api/v1/destinations")
      .then((destinations) => {
        if (!cancelled) setItems(destinations.items);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load destinations");
      });
    return () => {
      cancelled = true;
    };
  }, [location.state]);

  if (!items && !error) return <AdminLoading message="Loading destinations…" />;

  return (
    <div className="admin-content space-y-4">
      <AdminPageHeader
        title="Destinations"
        actions={isOwner ? <Link to="/destinations/new" className={primaryButtonClass}>Add destination</Link> : undefined}
      />
      {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
      {message && <p className="text-sm text-success-600" role="status">{message}</p>}
      {!isOwner && <p className="text-sm text-gray-500">Only the owner can add or change destinations.</p>}

      {items && items.length === 0 ? (
        <div className="admin-card">
          <AdminEmptyState icon={Cloud} title="No destinations yet" description="Add storage and backups can be copied off this machine." />
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {items?.map((destination) => {
            const check = checks[destination.id];
            const status = statusOf(destination, check);
            const needsSignIn = destination.provider === "telegram" && !destination.signed_in;
            const limit = destination.free_storage_bytes || 0;
            const used = destination.account_used_bytes || 0;
            return (
              <article key={destination.id} className={`admin-card ${needsSignIn ? "lg:col-span-2" : ""}`} aria-labelledby={`dest-${destination.id}`}>
                <div className="admin-card-body space-y-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="flex min-w-0 items-center gap-3">
                      <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-white">
                        <ProviderLogo id={destination.provider} className="h-8 w-8" />
                      </span>
                      <div className="min-w-0">
                        <h2 id={`dest-${destination.id}`} className="truncate text-base font-semibold text-gray-900 dark:text-white">{destination.name}</h2>
                        <p className="truncate text-sm text-gray-500">{destination.provider_name || destination.provider} · {detail(destination)}</p>
                      </div>
                    </div>
                    <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${status.className}`}>{status.label}</span>
                  </div>
                  <p className="text-sm text-gray-500">
                    {check ? (check.ok ? "Last check: connected just now" : `Last check: ${check.message}`) : "Last check: not yet"}
                  </p>
                  {limit > 0 && (
                    <p className="text-sm text-gray-500">{formatBytes(used)} of {formatCap(limit)} stored here{destination.allow_over_free ? ", and copying continues after the free allowance" : ""}</p>
                  )}
                  {isOwner && (
                    <div className="flex flex-wrap gap-2">
                      <button type="button" className={quietButtonClass} disabled={busy === destination.id} onClick={() => {
                        setBusy(destination.id);
                        setError(null);
                        void api(`/api/v1/destinations/${destination.id}/test`, { method: "POST" })
                          .then(() => {
                            setChecks((current) => ({ ...current, [destination.id]: { ok: true, message: "Connected" } }));
                            setMessage(`${destination.name} is connected.`);
                          })
                          .catch((err: unknown) => {
                            const text = err instanceof Error ? err.message : "Could not reach this storage";
                            setChecks((current) => ({ ...current, [destination.id]: { ok: false, message: text } }));
                            setError(text);
                          })
                          .finally(() => setBusy(null));
                      }}>
                        {busy === destination.id ? "Testing…" : "Test"}
                      </button>
                      <Link to={`/destinations/${destination.id}/edit`} className={quietButtonClass}>Edit</Link>
                      <button type="button" className={dangerButtonClass} onClick={() => {
                        const note = destination.provider === "telegram"
                          ? `Remove ${destination.name}? Backups stay on this machine. Messages already in the chat stay there.`
                          : `Remove ${destination.name}? Backups stay on this machine. Files already in the bucket stay there.`;
                        if (!confirm(note)) return;
                        setError(null);
                        void api(`/api/v1/destinations/${destination.id}`, { method: "DELETE" })
                          .then(load)
                          .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not remove destination"));
                      }}>
                        Delete
                      </button>
                      {destination.provider === "telegram" && destination.signed_in && (
                        <button type="button" className={quietButtonClass} onClick={() => {
                          if (!confirm(`Log out of Telegram for ${destination.name}?`)) return;
                          void api(`/api/v1/destinations/${destination.id}/telegram/logout`, { method: "POST" })
                            .then(() => {
                              setMessage(`${destination.name} logged out.`);
                              return load();
                            })
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not log out"));
                        }}>
                          Log out
                        </button>
                      )}
                    </div>
                  )}
                  {needsSignIn && isOwner && (
                    <TelegramSignIn
                      destinationID={destination.id}
                      name={destination.name}
                      onMessage={setMessage}
                      onDone={load}
                    />
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}
    </div>
  );
}
