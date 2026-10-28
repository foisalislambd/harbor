import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { ProviderLogo } from "@/components/provider-logo";
import { fieldClass, labelClass, primaryButtonClass } from "@/lib/fields";
import { api, formatBytes, type StorageProvider, type StorageProviderList } from "@/lib/harbor-api";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

export default function DestinationNewPage() {
  const { providerId = "" } = useParams();
  const navigate = useNavigate();
  const [provider, setProvider] = useState<StorageProvider | null>(null);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({
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
  });

  useEffect(() => {
    api<StorageProviderList>("/api/v1/destination-providers")
      .then((res) => {
        const found = res.items.find((item) => item.id === providerId);
        if (!found) {
          setMissing(true);
          return;
        }
        setProvider(found);
        setForm((current) => ({ ...current, name: current.name || found.name }));
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load this service"));
  }, [providerId]);

  async function createDestination(e: React.FormEvent) {
    e.preventDefault();
    if (!provider) return;
    setSaving(true);
    setError(null);
    try {
      await api("/api/v1/destinations", {
        method: "POST",
        body: JSON.stringify({ ...form, provider: provider.id }),
      });
      navigate("/destinations", { state: { message: `${provider.name} is connected. Choose it on a project to copy backups there.` } });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not add this destination");
      setSaving(false);
    }
  }

  if (!provider && !missing && !error) return <AdminLoading message="Loading service…" />;

  return (
    <div className="admin-content space-y-3">
      <AdminPageHeader
        title={provider?.name || "Service"}
        actions={<Link to="/destinations" className="inline-flex h-8 items-center justify-center rounded-lg border border-gray-200 px-3 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-white/5">Back</Link>}
      />
      {missing && <p className="text-sm text-error-500">That storage service is not available.</p>}
      {error && <p className="text-sm text-error-500">{error}</p>}
      {provider && (
        <section className="admin-card">
          <div className="admin-card-body space-y-4">
            <div className="flex items-center gap-3">
              <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-white">
                <ProviderLogo id={provider.id} className="h-8 w-8" />
              </span>
              <p className="text-sm text-gray-600 dark:text-gray-300">{provider.summary}</p>
            </div>
            {provider.free_note && <p className="text-sm text-gray-500">{provider.free_note}</p>}
            <ol className="list-decimal space-y-1 pl-5 text-sm text-gray-600 dark:text-gray-300">
              {provider.steps.map((step) => <li key={step}>{step}</li>)}
            </ol>
            <form onSubmit={createDestination} className="grid gap-4 md:grid-cols-2">
              <label className="block">
                <span className={labelClass}>Name</span>
                <input className={fieldClass} required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder={provider.name} />
              </label>
              <label className="block">
                <span className={labelClass}>Bucket</span>
                <input className={fieldClass} required value={form.bucket} onChange={(e) => setForm({ ...form, bucket: e.target.value })} placeholder="harbor-backups" />
              </label>
              {provider.account_id && (
                <label className="block md:col-span-2">
                  <span className={labelClass}>Account ID</span>
                  <input className={fieldClass} required value={form.account_id} onChange={(e) => setForm({ ...form, account_id: e.target.value })} placeholder="Account ID, or the full endpoint from the dashboard" />
                </label>
              )}
              {provider.region && (
                <label className="block">
                  <span className={labelClass}>Region</span>
                  <input className={fieldClass} required value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder={provider.region_placeholder} />
                </label>
              )}
              {provider.endpoint && (
                <label className={`block ${provider.region ? "" : "md:col-span-2"}`}>
                  <span className={labelClass}>Endpoint</span>
                  <input className={fieldClass} required value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder={provider.endpoint_placeholder} />
                </label>
              )}
              {provider.jurisdiction && (
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
              {provider.free_bytes > 0 && (
                <label className="flex items-center gap-2 self-end text-sm text-gray-600 dark:text-gray-300">
                  <input type="checkbox" checked={form.allow_over_free} onChange={(e) => setForm({ ...form, allow_over_free: e.target.checked })} />
                  Keep copying after the free {formatCap(provider.free_bytes)}
                </label>
              )}
              <div className="md:col-span-2">
                <button className={primaryButtonClass} disabled={saving} type="submit">{saving ? "Checking bucket…" : "Add destination"}</button>
              </div>
            </form>
          </div>
        </section>
      )}
    </div>
  );
}
