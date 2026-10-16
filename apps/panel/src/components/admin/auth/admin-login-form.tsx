import { useAdminAuth } from "@/components/admin/auth/admin-auth-provider";
import { adminConfig } from "@/config/admin.config";
import { fieldClass } from "@/lib/fields";
import { api } from "@/lib/harbor-api";
import { Eye, EyeOff, Lock, Mail, User } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useEffect, useState } from "react";

export function AdminLoginForm() {
  const { login, setup } = useAdminAuth();
  const navigate = useNavigate();
  const [needsSetup, setNeedsSetup] = useState<boolean | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api<{ needs_setup: boolean }>("/api/v1/setup")
      .then((res) => {
        if (!cancelled) setNeedsSetup(res.needs_setup);
      })
      .catch(() => {
        if (!cancelled) {
          setNeedsSetup(false);
          setError("Harbor API is not reachable. Start the server on port 8787.");
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      if (needsSetup) {
        await setup(name.trim(), email.trim(), password);
      } else {
        await login(email.trim(), password);
      }
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not sign in");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-dvh flex-col justify-center px-5 py-10 sm:px-10 lg:px-14 xl:px-16">
      <div className="mx-auto w-full max-w-[400px]">
        <div className="mb-6 flex h-11 w-11 items-center justify-center rounded-xl bg-brand-500 text-base font-bold text-white lg:hidden">
          {adminConfig.brand.letter}
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-[1.75rem]">
          {needsSetup ? "Create the owner account" : "Sign in"}
        </h1>
        <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
          {needsSetup
            ? "This Harbor instance has no users yet. The first account owns the vault."
            : "Open the vault that stores backups from all of your applications."}
        </p>

        <form onSubmit={handleSubmit} className="mt-8 space-y-5">
          {needsSetup && (
            <div>
              <label htmlFor="owner-name" className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
                Name
              </label>
              <div className="relative">
                <User className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-gray-400" />
                <input id="owner-name" value={name} onChange={(e) => setName(e.target.value)} required className={`${fieldClass} pl-10`} />
              </div>
            </div>
          )}
          <div>
            <label htmlFor="email" className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
              Email
            </label>
            <div className="relative">
              <Mail className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                id="email"
                type="email"
                autoComplete="username"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className={`${fieldClass} pl-10`}
              />
            </div>
          </div>
          <div>
            <label htmlFor="password" className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
              Password
            </label>
            <div className="relative">
              <Lock className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                id="password"
                type={showPassword ? "text" : "password"}
                autoComplete={needsSetup ? "new-password" : "current-password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
                className={`${fieldClass} pr-11 pl-10`}
              />
              <button
                type="button"
                onClick={() => setShowPassword((v) => !v)}
                className="absolute top-1/2 right-3 -translate-y-1/2 text-gray-400"
                aria-label={showPassword ? "Hide password" : "Show password"}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
          </div>
          {error && <p className="text-sm text-error-500">{error}</p>}
          <button
            type="submit"
            disabled={submitting || needsSetup === null}
            className="inline-flex h-11 w-full items-center justify-center rounded-lg bg-brand-500 text-sm font-semibold text-white shadow-sm shadow-brand-500/20 hover:bg-brand-600 disabled:opacity-60"
          >
            {submitting ? "Please wait…" : needsSetup ? "Create owner and continue" : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
}
