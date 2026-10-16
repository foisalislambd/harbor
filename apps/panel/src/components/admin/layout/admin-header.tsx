import { adminConfig } from "@/config/admin.config";
import { AdminUserMenu } from "@/components/admin/layout/admin-user-menu";
import { AdminThemeToggle } from "@/components/admin/ui/admin-theme-toggle";
import { useAdminSidebar } from "@/context/admin-sidebar-context";
import { Menu, Search, X } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

export function AdminHeader() {
  const { isMobileOpen, isDesktop, toggleMobileSidebar } = useAdminSidebar();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");

  return (
    <header className="admin-topbar sticky top-0 z-30 w-full bg-white dark:bg-gray-900">
      <div className="flex h-full items-center gap-3 px-3 sm:gap-4 sm:px-4">
        <button
          type="button"
          onClick={toggleMobileSidebar}
          className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-gray-200 text-gray-600 transition hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/30 dark:border-gray-800 dark:text-gray-400 dark:hover:bg-white/5 ${isDesktop ? "lg:hidden" : ""}`}
          aria-label={!isDesktop && isMobileOpen ? "Close menu" : "Open menu"}
          aria-expanded={!isDesktop ? isMobileOpen : undefined}
        >
          {!isDesktop && isMobileOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </button>

        <Link to="/" className="flex items-center gap-2 lg:hidden">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-500 text-sm font-bold text-white">
            {adminConfig.brand.letter}
          </span>
          <span className="text-base font-semibold text-gray-900 dark:text-white">{adminConfig.brand.name}</span>
        </Link>

        <form
          className="hidden min-w-0 flex-1 lg:block lg:max-w-lg"
          onSubmit={(e) => {
            e.preventDefault();
            const q = query.trim();
            navigate(q ? `/backups?q=${encodeURIComponent(q)}` : "/backups");
          }}
        >
          <label className="relative block">
            <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-gray-400" />
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search backups"
              className="h-9 w-full rounded-lg border border-gray-200 bg-gray-50 py-1.5 pr-4 pl-9 text-sm text-gray-800 outline-none placeholder:text-gray-400 focus:border-brand-300 focus:bg-white focus:ring-2 focus:ring-brand-500/20 dark:border-gray-800 dark:bg-white/5 dark:text-white"
              aria-label="Search backups"
            />
          </label>
        </form>

        <div className="ml-auto flex items-center gap-2 sm:gap-3">
          <AdminThemeToggle />
          <AdminUserMenu />
        </div>
      </div>
    </header>
  );
}
