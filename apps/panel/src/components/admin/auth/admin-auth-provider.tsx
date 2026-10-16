import { api, type SessionUser } from "@/lib/harbor-api";
import type { PublicAdmin } from "@/lib/admin-api";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

type AdminAuthContextValue = {
  admin: PublicAdmin | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  setup: (name: string, email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AdminAuthContext = createContext<AdminAuthContextValue | null>(null);

function toAdmin(user: SessionUser): PublicAdmin {
  return {
    id: user.id,
    username: user.name,
    email: user.email,
    role: user.role,
    lastLoginAt: null,
    createdAt: user.created_at,
  };
}

export function AdminAuthProvider({ children }: { children: ReactNode }) {
  const [admin, setAdmin] = useState<PublicAdmin | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    api<{ user: SessionUser }>("/api/v1/auth/me")
      .then((res) => {
        if (!cancelled) setAdmin(toAdmin(res.user));
      })
      .catch(() => {
        if (!cancelled) setAdmin(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const res = await api<{ user: SessionUser }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    setAdmin(toAdmin(res.user));
  }, []);

  const setup = useCallback(async (name: string, email: string, password: string) => {
    const res = await api<{ user: SessionUser }>("/api/v1/setup", {
      method: "POST",
      body: JSON.stringify({ name, email, password }),
    });
    setAdmin(toAdmin(res.user));
  }, []);

  const logout = useCallback(async () => {
    await api("/api/v1/auth/logout", { method: "POST" }).catch(() => undefined);
    setAdmin(null);
  }, []);

  const value = useMemo(
    () => ({ admin, loading, login, setup, logout }),
    [admin, loading, login, setup, logout],
  );

  return <AdminAuthContext.Provider value={value}>{children}</AdminAuthContext.Provider>;
}

export function useAdminAuth() {
  const ctx = useContext(AdminAuthContext);
  if (!ctx) {
    throw new Error("useAdminAuth must be used within AdminAuthProvider");
  }
  return ctx;
}
