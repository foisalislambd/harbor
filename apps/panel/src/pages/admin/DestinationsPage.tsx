import { useAdminAuth } from "@/components/admin/auth/admin-auth-provider";
import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { ProviderLogo } from "@/components/provider-logo";
import { dangerButtonClass, fieldClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Destination, type DestinationList, type StorageProvider, type StorageProviderList, type TelegramCode } from "@/lib/harbor-api";
import { Cloud } from "lucide-react";
import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from "react";
import { Link, useLocation } from "react-router-dom";

function formatCap(bytes: number) {
  if (bytes > 0 && bytes % 1_000_000_000 === 0) return `${bytes / 1_000_000_000} GB`;
  return formatBytes(bytes);
}

export default function DestinationsPage() {
  const { admin } = useAdminAuth();
  const isOwner = admin?.role === "owner";
  const location = useLocation();
  const [items, setItems] = useState<Destination[] | null>(null);
  const [providers, setProviders] = useState<StorageProvider[]>([]);
  const [error, setError] = useState<string | null>(null);
  const flash = (location.state as { message?: string } | null)?.message ?? null;
  const [message, setMessage] = useState<string | null>(flash);
  const [loginID, setLoginID] = useState<string | null>(null);
  const [loginPhase, setLoginPhase] = useState<"code" | "password" | "email" | "email_code">("code");
  const [loginCode, setLoginCode] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginEmail, setLoginEmail] = useState("");
  const [delivery, setDelivery] = useState<TelegramCode | null>(null);
  const [appliedFlash, setAppliedFlash] = useState(flash);
  if (flash !== appliedFlash) {
    setAppliedFlash(flash);
    if (flash) setMessage(flash);
  }

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
    let cancelled = false;
    Promise.all([
      api<DestinationList>("/api/v1/destinations"),
      api<StorageProviderList>("/api/v1/destination-providers"),
    ])
      .then(([destinations, catalog]) => {
        if (cancelled) return;
        setItems(destinations.items);
        setProviders(catalog.items);
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
    <div className="admin-content space-y-3">
      <AdminPageHeader title="Destinations" />
      {error && <p className="text-sm text-error-500">{error}</p>}
      {message && <p className="text-sm text-success-600">{message}</p>}

      {isOwner ? (
      <section className="admin-card">
        <div className="admin-card-body space-y-4">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Add storage</h2>
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
      ) : (
        <p className="text-sm text-gray-500">Only the owner can add or remove destinations.</p>
      )}

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
            const where = destination.provider === "telegram"
              ? [destination.account_id, destination.endpoint || "Saved Messages", destination.region && `topic ${destination.region}`, destination.signed_in ? (destination.account_label || "signed in") : "not signed in"].filter(Boolean).join(" · ")
              : [destination.bucket, destination.region, destination.endpoint, destination.jurisdiction && `${destination.jurisdiction} jurisdiction`].filter(Boolean).join(" · ");
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
                    {isOwner && (
                    <div className="flex gap-2">
                      <button
                        type="button"
                        className={quietButtonClass}
                        onClick={() => {
                          setError(null);
                          setMessage(null);
                          void api(`/api/v1/destinations/${destination.id}/test`, { method: "POST" })
                            .then(() => setMessage(destination.provider === "telegram" ? `${destination.name} is signed in.` : `${destination.name} can see the bucket.`))
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not reach this storage"));
                        }}
                      >
                        Test
                      </button>
                      <button
                        type="button"
                        className={dangerButtonClass}
                        onClick={() => {
                          const note = destination.provider === "telegram"
                            ? `Remove ${destination.name}? Backups stay on this machine. Messages already in the chat stay there.`
                            : `Remove ${destination.name}? Backups stay on this machine. Files already in the bucket stay there.`;
                          if (!confirm(note)) return;
                          setError(null);
                          void api(`/api/v1/destinations/${destination.id}`, { method: "DELETE" })
                            .then(load)
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not remove destination"));
                        }}
                      >
                        Remove
                      </button>
                    </div>
                    )}
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
                  {destination.provider === "telegram" && isOwner && (
                    <div className="space-y-2">
                      {destination.signed_in ? (
                        <button
                          type="button"
                          className={quietButtonClass}
                          onClick={() => {
                            if (!confirm(`Log out of Telegram for ${destination.name}? This revokes the session.`)) return;
                            setError(null);
                            void api(`/api/v1/destinations/${destination.id}/telegram/logout`, { method: "POST" })
                              .then(() => {
                                setMessage(`${destination.name} logged out of Telegram.`);
                                return load();
                              })
                              .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not log out"));
                          }}
                        >
                          Log out
                        </button>
                      ) : (
                        <TelegramLogin
                          destinationID={destination.id}
                          name={destination.name}
                          active={loginID === destination.id}
                          phase={loginPhase}
                          code={loginCode}
                          password={loginPassword}
                          email={loginEmail}
                          delivery={delivery}
                          onCode={setLoginCode}
                          onPassword={setLoginPassword}
                          onEmail={setLoginEmail}
                          onError={setError}
                          onMessage={setMessage}
                          onBegin={(status) => {
                            setLoginID(destination.id);
                            setDelivery(status);
                            setLoginCode("");
                            setLoginPassword("");
                            setLoginEmail("");
                            setLoginPhase(status.type === "setup_email" ? "email" : "code");
                          }}
                          onDelivery={(status) => {
                            setDelivery(status);
                            setLoginPhase(status.type === "setup_email" ? "email" : "code");
                          }}
                          onPasswordPhase={() => setLoginPhase("password")}
                          onEmailCode={() => setLoginPhase("email_code")}
                          onDone={() => {
                            setLoginID(null);
                            setDelivery(null);
                            return load();
                          }}
                          onCancel={() => {
                            setLoginID(null);
                            setDelivery(null);
                          }}
                        />
                      )}
                    </div>
                  )}
                  {destination.provider !== "telegram" && (
                    <p className="text-xs text-gray-500">Access key {destination.access_key_id} · prefix {destination.key_prefix}</p>
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

function TelegramLogin({
  destinationID,
  name,
  active,
  phase,
  code,
  password,
  email,
  delivery,
  onCode,
  onPassword,
  onEmail,
  onError,
  onMessage,
  onBegin,
  onDelivery,
  onPasswordPhase,
  onEmailCode,
  onDone,
  onCancel,
}: {
  destinationID: string;
  name: string;
  active: boolean;
  phase: "code" | "password" | "email" | "email_code";
  code: string;
  password: string;
  email: string;
  delivery: TelegramCode | null;
  onCode: (value: string) => void;
  onPassword: (value: string) => void;
  onEmail: (value: string) => void;
  onError: Dispatch<SetStateAction<string | null>>;
  onMessage: (value: string) => void;
  onBegin: (status: TelegramCode) => void;
  onDelivery: (status: TelegramCode) => void;
  onPasswordPhase: () => void;
  onEmailCode: () => void;
  onDone: () => Promise<void> | void;
  onCancel: () => void;
}) {
  const [localError, setLocalError] = useState<string | null>(null);
  const fail = (err: unknown, fallback: string) => {
    const text = err instanceof Error ? err.message : fallback;
    setLocalError(text);
    onError(text);
  };
  const clear = () => {
    setLocalError(null);
    onError(null);
  };
  const apply = (status: TelegramCode) => {
    onMessage(status.message);
    if (status.type === "already") {
      onCancel();
      return onDone();
    }
    if (!active) onBegin(status);
    else onDelivery(status);
  };
  return (
    <>
      <p className="text-xs text-gray-500">Telegram never delivers a login code to this new Harbor session. An in-app code arrives on a phone that is already signed in, in the chat named Telegram (777000).</p>
      {localError && <p className="text-sm text-error-500">{localError}</p>}
      {!active && (
        <button
          type="button"
          className={quietButtonClass}
          onClick={() => {
            clear();
            void api<TelegramCode>(`/api/v1/destinations/${destinationID}/telegram/code`, { method: "POST" })
              .then(apply)
              .catch((err: unknown) => fail(err, "Could not send a code"));
          }}
        >
          Send login code
        </button>
      )}
      {active && delivery && (
        <div className="space-y-2">
          <p className="text-sm text-gray-700 dark:text-gray-200">{delivery.message}</p>
          {delivery.url && <a className="text-sm text-brand-600 underline" href={delivery.url} target="_blank" rel="noreferrer">{delivery.url}</a>}
          {phase === "email" && (
            <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(event) => {
              event.preventDefault();
              clear();
              void api<{ email_pattern?: string }>(`/api/v1/destinations/${destinationID}/telegram/email`, { method: "POST", body: JSON.stringify({ email }) })
                .then((result) => {
                  onMessage(`Telegram sent a confirmation code to ${result.email_pattern || email}.`);
                  onEmailCode();
                })
                .catch((err: unknown) => fail(err, "Could not send the email code"));
            }}>
              <input className={fieldClass} type="email" value={email} onChange={(e) => onEmail(e.target.value)} placeholder="Login email" required />
              <button className={quietButtonClass} type="submit">Send email code</button>
            </form>
          )}
          {phase === "email_code" && (
            <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(event) => {
              event.preventDefault();
              clear();
              void api<TelegramCode>(`/api/v1/destinations/${destinationID}/telegram/email/verify`, { method: "POST", body: JSON.stringify({ code }) })
                .then((status) => {
                  onCode("");
                  apply(status);
                })
                .catch((err: unknown) => fail(err, "Could not confirm the email"));
            }}>
              <input className={fieldClass} value={code} onChange={(e) => onCode(e.target.value)} placeholder="Email code" required />
              <button className={quietButtonClass} type="submit">Confirm email</button>
            </form>
          )}
          {(phase === "code" || phase === "password") && delivery.type !== "setup_email" && delivery.type !== "payment" && (
            <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(event) => {
              event.preventDefault();
              clear();
              const body = phase === "password" ? { password } : { code, password };
              void api<{ status: string }>(`/api/v1/destinations/${destinationID}/telegram/login`, { method: "POST", body: JSON.stringify(body) })
                .then((result) => {
                  if (result.status === "password") {
                    onPasswordPhase();
                    onMessage("This account has a cloud password.");
                    return;
                  }
                  onMessage(`${name} is signed in.`);
                  return onDone();
                })
                .catch((err: unknown) => fail(err, "Could not sign in"));
            }}>
              {phase === "code" ? (
                <input className={fieldClass} value={code} onChange={(e) => onCode(e.target.value)} placeholder="Login code" required />
              ) : (
                <input className={fieldClass} type="password" value={password} onChange={(e) => onPassword(e.target.value)} placeholder="Cloud password" required />
              )}
              <button className={quietButtonClass} type="submit">{phase === "password" ? "Confirm password" : "Confirm code"}</button>
            </form>
          )}
          <div className="flex flex-wrap gap-2">
            {delivery.next_type && phase !== "email" && phase !== "email_code" && (
              <button type="button" className={quietButtonClass} onClick={() => {
                clear();
                void api<TelegramCode>(`/api/v1/destinations/${destinationID}/telegram/resend`, { method: "POST" })
                  .then(apply)
                  .catch((err: unknown) => fail(err, "Could not resend the code"));
              }}>
                Resend by {delivery.next_type.replaceAll("_", " ")}
              </button>
            )}
            <button type="button" className={quietButtonClass} onClick={() => {
              clear();
              void api(`/api/v1/destinations/${destinationID}/telegram/cancel`, { method: "POST" })
                .then(() => {
                  onMessage(`Login cancelled for ${name}.`);
                  onCancel();
                })
                .catch((err: unknown) => fail(err, "Could not cancel the code"));
            }}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </>
  );
}
