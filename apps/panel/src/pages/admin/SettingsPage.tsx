import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { useAdminAuth } from "@/components/admin/auth/admin-auth-provider";
import { dangerButtonClass, fieldClass, labelClass, primaryButtonClass } from "@/lib/fields";
import { api, formatBytes, type SessionUser, type Settings } from "@/lib/harbor-api";
import { useEffect, useState } from "react";

export default function SettingsPage() {
  const { admin } = useAdminAuth();
  const isOwner = admin?.role === "owner";
  const [settings, setSettings] = useState<Settings | null>(null);
  const [users, setUsers] = useState<SessionUser[]>([]);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [nextPassword, setNextPassword] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    const [settingsRes, usersRes] = await Promise.all([
      api<Settings>("/api/v1/settings"),
      api<{ items: SessionUser[] }>("/api/v1/users"),
    ]);
    setSettings(settingsRes);
    setUsers(usersRes.items);
  }

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api<Settings>("/api/v1/settings"),
      api<{ items: SessionUser[] }>("/api/v1/users"),
    ])
      .then(([settingsRes, usersRes]) => {
        if (cancelled) return;
        setSettings(settingsRes);
        setUsers(usersRes.items);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load settings");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!settings && !error) return <AdminLoading message="Loading settings…" />;

  return (
    <div className="admin-content space-y-3">
      <AdminPageHeader title="Settings" />
      {error && <p className="text-sm text-error-500">{error}</p>}
      {message && <p className="text-sm text-success-600">{message}</p>}

      {settings && (
        <form
          className="admin-card"
          onSubmit={(e) => {
            e.preventDefault();
            setError(null);
            void api("/api/v1/settings", {
              method: "PUT",
              body: JSON.stringify({ instance_name: settings.instance_name }),
            })
              .then(() => setMessage("Instance name saved"))
              .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not save settings"));
          }}
        >
          <div className="admin-card-body space-y-4">
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Instance</h2>
            <label className="block max-w-md">
              <span className={labelClass}>Name</span>
              <input className={fieldClass} value={settings.instance_name} onChange={(e) => setSettings({ ...settings, instance_name: e.target.value })} />
            </label>
            <p className="text-sm text-gray-500">
              Version {settings.version} · data directory {settings.data_dir} · max upload {formatBytes(settings.max_upload_bytes)}
            </p>
            <button className={primaryButtonClass} type="submit">Save</button>
          </div>
        </form>
      )}

      <form
        className="admin-card"
        onSubmit={(e) => {
          e.preventDefault();
          setError(null);
          void api("/api/v1/auth/password", {
            method: "POST",
            body: JSON.stringify({ current_password: currentPassword, new_password: nextPassword }),
          })
            .then(() => {
              setCurrentPassword("");
              setNextPassword("");
              setMessage("Password updated");
            })
            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not update password"));
        }}
      >
        <div className="admin-card-body space-y-4">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Your password</h2>
          <div className="grid max-w-xl gap-3 md:grid-cols-2">
            <input className={fieldClass} type="password" placeholder="Current password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} required />
            <input className={fieldClass} type="password" placeholder="New password" minLength={8} value={nextPassword} onChange={(e) => setNextPassword(e.target.value)} required />
          </div>
          <button className={primaryButtonClass} type="submit">Update password</button>
        </div>
      </form>

      <section className="admin-card">
        <div className="admin-card-body space-y-4">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Team</h2>
          {isOwner && (
          <form
            className="grid gap-3 md:grid-cols-4"
            onSubmit={(e) => {
              e.preventDefault();
              setError(null);
              void api("/api/v1/users", {
                method: "POST",
                body: JSON.stringify({ name, email, password }),
              }).then(() => {
                setName("");
                setEmail("");
                setPassword("");
                return load();
              }).catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not add user"));
            }}
          >
            <input className={fieldClass} placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} required />
            <input className={fieldClass} type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            <input className={fieldClass} type="password" placeholder="Password" minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} required />
            <button className={primaryButtonClass} type="submit">Add user</button>
          </form>
          )}
          <ul className="divide-y divide-gray-100 dark:divide-gray-800">
            {users.map((user) => (
              <li key={user.id} className="flex items-center justify-between py-3 text-sm">
                <span>
                  <span className="font-medium text-gray-900 dark:text-white">{user.name}</span>
                  <span className="text-gray-500"> · {user.email} · {user.role}</span>
                </span>
                {isOwner && user.id !== admin?.id && (
                  <button
                    type="button"
                    className={dangerButtonClass}
                    onClick={() => {
                      setError(null);
                      void api(`/api/v1/users/${user.id}`, { method: "DELETE" })
                        .then(load)
                        .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not remove user"));
                    }}
                  >
                    Remove
                  </button>
                )}
              </li>
            ))}
          </ul>
        </div>
      </section>
    </div>
  );
}
