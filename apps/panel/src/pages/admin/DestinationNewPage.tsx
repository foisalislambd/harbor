import { useAdminAuth } from "@/components/admin/auth/admin-auth-provider";
import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { ProviderLogo } from "@/components/provider-logo";
import { fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Destination, type StorageProvider, type StorageProviderList } from "@/lib/harbor-api";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

type FormState = {
  name: string;
  account_id: string;
  access_key_id: string;
  secret_access_key: string;
  bucket: string;
  jurisdiction: string;
  region: string;
  endpoint: string;
  key_prefix: string;
  allow_over_free: boolean;
};

const emptyForm: FormState = {
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

function problems(provider: StorageProvider, form: FormState, editing: boolean) {
  const next: Record<string, string> = {};
  if (!form.name.trim()) next.name = "Give this destination a name.";
  if (provider.telegram) {
    if (!/^\d{3,12}$/.test(form.access_key_id.trim())) next.access_key_id = "Use the numeric API ID from my.telegram.org.";
    if (!editing && !/^[0-9a-fA-F]{16,64}$/.test(form.secret_access_key.trim())) next.secret_access_key = "Paste the API hash from the same page.";
    if (editing && form.secret_access_key && !/^[0-9a-fA-F]{16,64}$/.test(form.secret_access_key.trim())) next.secret_access_key = "That does not look like an API hash.";
    const phone = form.account_id.replace(/[\s()-]/g, "");
    if (!/^\+?\d{7,15}$/.test(phone)) next.account_id = "Use the international number, such as +15551234567.";
  } else {
    if (!/^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/.test(form.bucket.trim().toLowerCase())) next.bucket = "Use 3 to 63 lowercase letters, numbers, dots, or hyphens.";
    if (provider.account_id && !form.account_id.trim()) next.account_id = "Account ID is required.";
    if (provider.region && !form.region.trim()) next.region = "Region is required.";
    if (provider.endpoint && !form.endpoint.trim()) next.endpoint = "Endpoint is required.";
    if (!form.access_key_id.trim()) next.access_key_id = "Access key is required.";
    if (!editing && form.secret_access_key.trim().length < 8) next.secret_access_key = "Secret key is required.";
  }
  return next;
}

export default function DestinationNewPage() {
  const { providerId = "", destinationId = "" } = useParams();
  const editing = destinationId !== "";
  const navigate = useNavigate();
  const { admin, loading: authLoading } = useAdminAuth();
  const [providers, setProviders] = useState<StorageProvider[]>([]);
  const [provider, setProvider] = useState<StorageProvider | null>(null);
  const [query, setQuery] = useState("");
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!authLoading && admin && admin.role !== "owner") {
      navigate("/destinations", { replace: true });
    }
  }, [admin, authLoading, navigate]);

  useEffect(() => {
    let live = true;
    api<StorageProviderList>("/api/v1/destination-providers")
      .then(async (catalog) => {
        if (!live) return;
        setProviders(catalog.items);
        if (editing) {
          const current = await api<Destination>(`/api/v1/destinations/${destinationId}`);
          if (!live) return;
          const found = catalog.items.find((item) => item.id === current.provider);
          if (!found) {
            setMissing(true);
            return;
          }
          setProvider(found);
          setForm({
            ...emptyForm,
            name: current.name,
            account_id: current.account_id,
            access_key_id: current.access_key_id,
            bucket: current.bucket,
            jurisdiction: current.jurisdiction,
            region: current.region,
            endpoint: current.endpoint,
            key_prefix: current.key_prefix || "harbor",
            allow_over_free: current.allow_over_free,
          });
        } else if (providerId) {
          const found = catalog.items.find((item) => item.id === providerId);
          if (!found) setMissing(true);
          else {
            setProvider(found);
            setForm((current) => ({ ...current, name: current.name || found.name }));
          }
        }
        setReady(true);
      })
      .catch((err: unknown) => {
        if (live) setError(err instanceof Error ? err.message : "Could not load storage services");
      });
    return () => {
      live = false;
    };
  }, [providerId, destinationId, editing]);

  const groups = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const visible = providers.filter((item) => !needle || item.name.toLowerCase().includes(needle) || item.group.toLowerCase().includes(needle));
    const order: string[] = [];
    for (const item of visible) if (!order.includes(item.group)) order.push(item.group);
    return order.map((group) => ({ group, items: visible.filter((item) => item.group === group) }));
  }, [providers, query]);

  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (!provider) return;
    const found = problems(provider, form, editing);
    setFieldErrors(found);
    if (Object.keys(found).length > 0) return;
    setSaving(true);
    setError(null);
    const payload = { ...form, provider: provider.id, region: provider.telegram ? form.region : form.region };
    try {
      if (editing) {
        await api(`/api/v1/destinations/${destinationId}`, { method: "PATCH", body: JSON.stringify(payload) });
        navigate("/destinations", { state: { message: `${form.name} is saved.` } });
      } else {
        await api("/api/v1/destinations", { method: "POST", body: JSON.stringify(payload) });
        navigate("/destinations", { state: { message: provider.telegram ? `${form.name} is saved. Scan the code on this page to sign in.` : `${form.name} is connected.` } });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save this destination");
      setSaving(false);
    }
  }

  if (!ready && !error) return <AdminLoading message="Loading…" />;

  if (!providerId && !editing) {
    return (
      <div className="admin-content space-y-4">
        <AdminPageHeader
          title="Add destination"
          actions={<Link to="/destinations" className={quietButtonClass}>Back</Link>}
        />
        <label className="block max-w-md">
          <span className="sr-only">Search services</span>
          <input className={fieldClass} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search services" />
        </label>
        {groups.length === 0 && <p className="text-sm text-gray-500">No services match that search.</p>}
        {groups.map((group) => (
          <section key={group.group} className="space-y-2">
            <h2 className="text-sm font-medium text-gray-500">{group.group}</h2>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4">
              {group.items.map((item) => (
                <Link key={item.id} to={`/destinations/new/${item.id}`} className="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-3 text-left hover:border-brand-300 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-white/5">
                  <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-white">
                    <ProviderLogo id={item.id} />
                  </span>
                  <span className="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100">{item.name}</span>
                </Link>
              ))}
            </div>
          </section>
        ))}
      </div>
    );
  }

  return (
    <div className="admin-content space-y-4">
      <AdminPageHeader
        title={editing ? `Edit ${form.name || "destination"}` : provider?.name || "Destination"}
        actions={<Link to="/destinations" className={quietButtonClass}>Back</Link>}
      />
      {missing && <p className="text-sm text-error-500">That destination is not available.</p>}
      {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
      {provider && (
        <section className="admin-card">
          <form onSubmit={save} className="admin-card-body space-y-4" noValidate>
            <div className="flex items-center gap-3">
              <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-white">
                <ProviderLogo id={provider.id} className="h-8 w-8" />
              </span>
              <p className="text-sm text-gray-600 dark:text-gray-300">{provider.summary}</p>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="Name" error={fieldErrors.name} className="md:col-span-2">
                <input className={fieldClass} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder={provider.name} autoComplete="off" />
              </Field>
              {provider.telegram ? (
                <>
                  <Field label="API ID" error={fieldErrors.access_key_id} hint="The number from my.telegram.org/apps">
                    <input className={fieldClass} inputMode="numeric" value={form.access_key_id} onChange={(e) => setForm({ ...form, access_key_id: e.target.value })} placeholder="123456" autoComplete="off" />
                  </Field>
                  <Field label="Phone number" error={fieldErrors.account_id}>
                    <input className={fieldClass} value={form.account_id} onChange={(e) => setForm({ ...form, account_id: e.target.value })} placeholder="+15551234567" autoComplete="tel" />
                  </Field>
                  <Field label="API hash" error={fieldErrors.secret_access_key} className="md:col-span-2" hint={editing ? "Leave blank to keep the saved hash." : "Shown once on my.telegram.org/apps"}>
                    <input className={fieldClass} type="password" autoComplete="new-password" value={form.secret_access_key} onChange={(e) => setForm({ ...form, secret_access_key: e.target.value })} placeholder={editing ? "Unchanged" : "API hash"} />
                  </Field>
                  <Field label="Where to send backups" error={fieldErrors.endpoint} className="md:col-span-2" hint="Leave blank for Saved Messages.">
                    <input className={fieldClass} value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder="@channel, invite link, or leave blank" />
                  </Field>
                </>
              ) : (
                <>
                  <Field label="Bucket" error={fieldErrors.bucket}>
                    <input className={fieldClass} value={form.bucket} onChange={(e) => setForm({ ...form, bucket: e.target.value })} placeholder="harbor-backups" autoComplete="off" />
                  </Field>
                  {provider.account_id && (
                    <Field label="Account ID" error={fieldErrors.account_id} className="md:col-span-2">
                      <input className={fieldClass} value={form.account_id} onChange={(e) => setForm({ ...form, account_id: e.target.value })} placeholder={provider.endpoint_placeholder || "Account ID"} />
                    </Field>
                  )}
                  {provider.region && (
                    <Field label="Region" error={fieldErrors.region}>
                      <input className={fieldClass} value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder={provider.region_placeholder || "us-east-1"} />
                    </Field>
                  )}
                  {provider.endpoint && (
                    <Field label="Endpoint" error={fieldErrors.endpoint} className={provider.region ? "" : "md:col-span-2"}>
                      <input className={fieldClass} value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder={provider.endpoint_placeholder || "https://s3.example.com"} />
                    </Field>
                  )}
                  <Field label="Access key" error={fieldErrors.access_key_id}>
                    <input className={fieldClass} autoComplete="off" value={form.access_key_id} onChange={(e) => setForm({ ...form, access_key_id: e.target.value })} placeholder="Access key" />
                  </Field>
                  <Field label="Secret key" error={fieldErrors.secret_access_key} hint={editing ? "Leave blank to keep the saved key." : undefined}>
                    <input className={fieldClass} type="password" autoComplete="new-password" value={form.secret_access_key} onChange={(e) => setForm({ ...form, secret_access_key: e.target.value })} placeholder={editing ? "Unchanged" : "Secret key"} />
                  </Field>
                </>
              )}
            </div>
            <details className="rounded-lg border border-gray-200 px-3 py-2 dark:border-gray-700">
              <summary className="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">More options</summary>
              <div className="mt-3 grid gap-4 md:grid-cols-2">
                {provider.telegram ? (
                  <Field label="Forum topic" error={fieldErrors.region} hint="Only for a forum chat. Leave blank otherwise.">
                    <input className={fieldClass} value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder="Topic number" />
                  </Field>
                ) : (
                  <>
                    <Field label="Key prefix">
                      <input className={fieldClass} value={form.key_prefix} onChange={(e) => setForm({ ...form, key_prefix: e.target.value })} placeholder="harbor" />
                    </Field>
                    {provider.jurisdiction && (
                      <Field label="Jurisdiction">
                        <select className={fieldClass} value={form.jurisdiction} onChange={(e) => setForm({ ...form, jurisdiction: e.target.value })}>
                          <option value="">Default</option>
                          <option value="eu">European Union</option>
                          <option value="fedramp">FedRAMP</option>
                        </select>
                      </Field>
                    )}
                    {provider.free_bytes > 0 && (
                      <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200 md:col-span-2">
                        <input type="checkbox" checked={form.allow_over_free} onChange={(e) => setForm({ ...form, allow_over_free: e.target.checked })} />
                        Keep copying after the free {formatCap(provider.free_bytes)}
                      </label>
                    )}
                  </>
                )}
              </div>
            </details>
            <button className={primaryButtonClass} disabled={saving} type="submit">{saving ? "Saving…" : editing ? "Save changes" : "Add destination"}</button>
          </form>
        </section>
      )}
    </div>
  );
}

function Field({ label, error, hint, className, children }: { label: string; error?: string; hint?: string; className?: string; children: ReactNode }) {
  return (
    <label className={`block ${className || ""}`}>
      <span className={labelClass}>{label}</span>
      {children}
      {hint && !error && <span className="mt-1 block text-xs text-gray-500">{hint}</span>}
      {error && <span className="mt-1 block text-xs text-error-500" role="alert">{error}</span>}
    </label>
  );
}
