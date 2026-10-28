import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { dangerButtonClass, fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Destination, type DestinationList } from "@/lib/harbor-api";
import { Cloud } from "lucide-react";
import { useEffect, useState } from "react";

const emptyForm = {
  name: "",
  account_id: "",
  access_key_id: "",
  secret_access_key: "",
  bucket: "",
  jurisdiction: "",
  key_prefix: "harbor",
};

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

export default function DestinationsPage() {
  const [items, setItems] = useState<Destination[] | null>(null);
  const [freeBytes, setFreeBytes] = useState(10_000_000_000);
  const [freeClassA, setFreeClassA] = useState(1_000_000);
  const [freeClassB, setFreeClassB] = useState(10_000_000);
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function load() {
    const res = await api<DestinationList>("/api/v1/destinations");
    setItems(res.items);
    setFreeBytes(res.free_storage_bytes);
    setFreeClassA(res.free_class_a);
    setFreeClassB(res.free_class_b);
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
        body: JSON.stringify({ ...form, provider: "cloudflare_r2" }),
      });
      setForm(emptyForm);
      setMessage("Cloudflare R2 is connected. Choose it on a project to copy backups there.");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not add Cloudflare");
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
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Cloudflare R2</h2>
          <p className="max-w-2xl text-sm leading-relaxed text-gray-500">
            Backups always stay on this machine. Add Cloudflare when a project should also keep a second copy.
            The free plan includes {formatCap(freeBytes)} of Standard storage, {freeClassA.toLocaleString()} writes, and {freeClassB.toLocaleString()} reads each month. Egress is free.
            Harbor stops new copies at {formatCap(freeBytes)} so this stays inside that storage. Infrequent Access is not included, so leave the bucket on Standard.
            Other storage services can be added here later.
          </p>
          <ol className="list-decimal space-y-1 pl-5 text-sm text-gray-600 dark:text-gray-300">
            <li>In the Cloudflare dashboard, open R2 and create a bucket.</li>
            <li>On R2 Overview, choose Manage API tokens, then Create Account API token.</li>
            <li>Give it Object Read & Write, and apply it to that bucket only.</li>
            <li>Copy the Access Key ID, the Secret Access Key, and the account ID from the S3 endpoint.</li>
          </ol>
          <form onSubmit={createDestination} className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className={labelClass}>Name</span>
              <input className={fieldClass} required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Cloudflare" />
            </label>
            <label className="block">
              <span className={labelClass}>Bucket</span>
              <input className={fieldClass} required value={form.bucket} onChange={(e) => setForm({ ...form, bucket: e.target.value })} placeholder="harbor-backups" />
            </label>
            <label className="block md:col-span-2">
              <span className={labelClass}>Account ID</span>
              <input className={fieldClass} required value={form.account_id} onChange={(e) => setForm({ ...form, account_id: e.target.value })} placeholder="32-character account ID, or the full R2 endpoint" />
            </label>
            <label className="block">
              <span className={labelClass}>Access Key ID</span>
              <input className={fieldClass} required autoComplete="off" value={form.access_key_id} onChange={(e) => setForm({ ...form, access_key_id: e.target.value })} />
            </label>
            <label className="block">
              <span className={labelClass}>Secret Access Key</span>
              <input className={fieldClass} required type="password" autoComplete="new-password" value={form.secret_access_key} onChange={(e) => setForm({ ...form, secret_access_key: e.target.value })} />
            </label>
            <label className="block">
              <span className={labelClass}>Jurisdiction</span>
              <select className={fieldClass} value={form.jurisdiction} onChange={(e) => setForm({ ...form, jurisdiction: e.target.value })}>
                <option value="">Default</option>
                <option value="eu">European Union</option>
                <option value="fedramp">FedRAMP</option>
              </select>
            </label>
            <label className="block">
              <span className={labelClass}>Key prefix</span>
              <input className={fieldClass} value={form.key_prefix} onChange={(e) => setForm({ ...form, key_prefix: e.target.value })} />
            </label>
            <div className="md:col-span-2">
              <button className={primaryButtonClass} disabled={saving} type="submit">{saving ? "Checking bucket…" : "Add Cloudflare"}</button>
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
            const limit = destination.free_storage_bytes || freeBytes;
            const accountUsed = destination.account_used_bytes || 0;
            const width = limit > 0 ? Math.min(100, (Math.min(limit, accountUsed) / limit) * 100) : 0;
            return (
              <article key={destination.id} className="admin-card">
                <div className="admin-card-body space-y-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h2 className="text-base font-semibold text-gray-900 dark:text-white">{destination.name}</h2>
                      <p className="text-sm text-gray-500">Cloudflare R2 · {destination.bucket} · {destination.jurisdiction || "default"} jurisdiction</p>
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
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not reach Cloudflare"));
                        }}
                      >
                        Test
                      </button>
                      <button
                        type="button"
                        className={dangerButtonClass}
                        onClick={() => {
                          if (!confirm(`Remove ${destination.name}? Backups stay on this machine. Files already in the bucket stay in Cloudflare.`)) return;
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
                  <p className="text-sm text-gray-500">{formatBytes(accountUsed)} of {formatCap(limit)} on this Cloudflare account</p>
                  <div className="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800">
                    <div className="h-full bg-brand-500" style={{ width: `${width}%` }} />
                  </div>
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
