import { adminNavItems, type AdminNavItem } from "@/config/admin.config";

export function normalizePathname(pathname: string): string {
  if (pathname.length > 1 && pathname.endsWith("/")) {
    return pathname.replace(/\/+$/, "");
  }
  return pathname;
}

export function matchAdminNavItem(pathname: string): AdminNavItem | undefined {
  const path = normalizePathname(pathname);
  const sorted = [...adminNavItems].sort((a, b) => b.href.length - a.href.length);
  return sorted.find((item) => isAdminNavActive(path, item.href));
}

export function isAdminNavActive(pathname: string, href: string) {
  const path = normalizePathname(pathname);
  if (href === "/") return path === "/";
  return path === href || path.startsWith(`${href}/`);
}

export function isExternalUrl(url: string): boolean {
  return /^https?:\/\//i.test(url);
}
