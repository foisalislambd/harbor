import { fieldClass, quietButtonClass } from "@/lib/fields";
import { api, type TelegramCode, type TelegramQR } from "@/lib/harbor-api";
import { X } from "lucide-react";
import { useEffect, useId, useState } from "react";

export function TelegramSignIn({
  destinationID,
  name,
  onClose,
  onDone,
  onMessage,
}: {
  destinationID: string;
  name: string;
  onClose: () => void;
  onDone: () => Promise<void> | void;
  onMessage: (value: string) => void;
}) {
  const [qr, setQR] = useState<TelegramQR | null>(null);
  const [delivery, setDelivery] = useState<TelegramCode | null>(null);
  const [phase, setPhase] = useState<"qr" | "password" | "email" | "email_code" | "code">("qr");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  const titleID = useId();

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = previous;
    };
  }, [onClose]);

  const fail = (err: unknown, fallback: string) => {
    setError(err instanceof Error ? err.message : fallback);
  };

  function applyQR(status: TelegramQR) {
    if (status.status === "ok") {
      onMessage(`${status.account_label || name} is signed in.`);
      return onDone();
    }
    if (status.status === "password") {
      setPhase("password");
      setQR(status);
      return;
    }
    setPhase("qr");
    setQR(status);
  }

  useEffect(() => {
    let live = true;
    void api<TelegramQR>(`/api/v1/destinations/${destinationID}/telegram/qr`, { method: "POST" })
      .then((status) => {
        if (live) applyQR(status);
      })
      .catch((err: unknown) => {
        if (live) fail(err, "Could not start sign-in");
      });
    return () => {
      live = false;
    };
    // Start once when this destination is shown.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [destinationID]);

  useEffect(() => {
    if (phase !== "qr" || qr?.status !== "qr") return;
    let live = true;
    const poll = () => {
      void api<TelegramQR>(`/api/v1/destinations/${destinationID}/telegram/qr/poll`, { method: "POST" })
        .then((status) => {
          if (live) applyQR(status);
        })
        .catch((err: unknown) => {
          if (live) fail(err, "Could not refresh the code");
        });
    };
    const id = window.setInterval(poll, 2000);
    return () => {
      live = false;
      window.clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [destinationID, phase, qr?.status]);

  useEffect(() => {
    if (!qr?.expires) return;
    const id = window.setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000);
    return () => window.clearInterval(id);
  }, [qr?.expires]);

  const left = qr?.expires ? Math.max(0, qr.expires - now) : qr?.expires_in ?? 0;

  async function run(work: () => Promise<void>, fallback: string) {
    if (pending) return;
    setPending(true);
    setError(null);
    try {
      await work();
    } catch (err) {
      fail(err, fallback);
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-gray-950/40 p-4 sm:items-center" onClick={onClose}>
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleID}
      className="admin-card admin-dialog max-h-[90dvh] w-full max-w-md overflow-y-auto"
      onClick={(event) => event.stopPropagation()}
    >
    <div className="admin-card-body space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 id={titleID} className="text-lg font-semibold text-gray-900 dark:text-white">Sign in</h3>
          <p className="mt-1 text-sm text-gray-500">{name}</p>
        </div>
        <button type="button" onClick={onClose} aria-label="Close" className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/10">
          <X className="h-4 w-4" />
        </button>
      </div>
      <ol className="list-decimal space-y-1 pl-5 text-sm text-gray-600 dark:text-gray-300">
        <li>Open Telegram on your phone</li>
        <li>Go to Settings, Devices, Link Desktop Device</li>
        <li>Scan this code</li>
      </ol>
      {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
      {phase === "password" ? (
        <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(event) => {
          event.preventDefault();
          void run(async () => {
            await api(`/api/v1/destinations/${destinationID}/telegram/login`, { method: "POST", body: JSON.stringify({ password }) });
            onMessage(`${name} is signed in.`);
            await onDone();
          }, "Could not sign in");
        }}>
          <label className="min-w-0 flex-1">
            <span className="sr-only">Cloud password</span>
            <input className={fieldClass} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Cloud password" required autoComplete="current-password" />
          </label>
          <button className={quietButtonClass} type="submit" disabled={pending}>Continue</button>
        </form>
      ) : qr?.image ? (
        <div className="flex flex-col items-center gap-2">
          <img src={qr.image} alt="QR code to link this Telegram account" className="h-64 w-64 max-w-full rounded-lg bg-white p-3" />
          <p className="text-sm text-gray-500" aria-live="polite">Refreshes in {left}s</p>
        </div>
      ) : (
        <p className="text-sm text-gray-500">{error ? "Try again in a moment." : "Preparing the code…"}</p>
      )}
      <details className="text-sm">
        <summary className="cursor-pointer font-medium text-gray-700 dark:text-gray-200">Use phone code instead</summary>
        <div className="mt-3 space-y-3">
          {!delivery && (
            <button type="button" className={quietButtonClass} disabled={pending} onClick={() => {
              void run(async () => {
                const status = await api<TelegramCode>(`/api/v1/destinations/${destinationID}/telegram/code`, { method: "POST" });
                setDelivery(status);
                setPhase(status.type === "setup_email" ? "email" : "code");
              }, "Could not send a code");
            }}>
              Send code
            </button>
          )}
          {delivery && <p className="text-sm text-gray-700 dark:text-gray-200">{delivery.message}</p>}
          {delivery?.url && <a className="text-sm text-brand-600 underline" href={delivery.url} target="_blank" rel="noreferrer">Open the link</a>}
          {phase === "email" && (
            <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(event) => {
              event.preventDefault();
              void run(async () => {
                await api(`/api/v1/destinations/${destinationID}/telegram/email`, { method: "POST", body: JSON.stringify({ email }) });
                setPhase("email_code");
                setDelivery({ ...delivery!, message: "Enter the code from your email." });
              }, "Could not send the email");
            }}>
              <input className={fieldClass} type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="Email address" />
              <button className={quietButtonClass} type="submit" disabled={pending}>Send</button>
            </form>
          )}
          {(phase === "code" || phase === "email_code") && delivery && (
            <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(event) => {
              event.preventDefault();
              void run(async () => {
                if (phase === "email_code") {
                  const status = await api<TelegramCode>(`/api/v1/destinations/${destinationID}/telegram/email/verify`, { method: "POST", body: JSON.stringify({ code }) });
                  setCode("");
                  setDelivery(status);
                  setPhase(status.type === "setup_email" ? "email" : "code");
                  return;
                }
                const result = await api<{ status: string }>(`/api/v1/destinations/${destinationID}/telegram/login`, { method: "POST", body: JSON.stringify({ code }) });
                if (result.status === "password") {
                  setPhase("password");
                  return;
                }
                onMessage(`${name} is signed in.`);
                await onDone();
              }, "Could not sign in");
            }}>
              <input className={fieldClass} value={code} onChange={(e) => setCode(e.target.value)} placeholder="Code" required autoComplete="one-time-code" />
              <button className={quietButtonClass} type="submit" disabled={pending}>Continue</button>
            </form>
          )}
        </div>
      </details>
    </div>
    </div>
    </div>
  );
}
