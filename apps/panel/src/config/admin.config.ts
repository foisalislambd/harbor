import type { LucideIcon } from "lucide-react";
import {
  Activity,
  Archive,
  BookOpen,
  FolderKanban,
  LayoutDashboard,
  Settings,
} from "lucide-react";

export const adminConfig = {
  brand: {
    name: "Harbor",
    tagline: "Self-hosted backups",
    letter: "H",
    siteUrl: "",
    loginDescription:
      "One place for every application's backups. Run it on your PC, a VPS, or in Docker.",
    loginFeatures: [
      "Separate projects and API keys",
      "Archive, database, and file snapshots",
      "Retention, quotas, and restore downloads",
    ],
  },
};

export type AdminNavItem = {
  name: string;
  href: string;
  icon: LucideIcon;
  description?: string;
  section?: string;
};

export const adminNavItems: AdminNavItem[] = [
  { name: "Dashboard", href: "/", icon: LayoutDashboard, section: "Overview" },
  {
    name: "Projects",
    href: "/projects",
    icon: FolderKanban,
    section: "Vault",
    description: "Applications that send backups",
  },
  {
    name: "Backups",
    href: "/backups",
    icon: Archive,
    section: "Vault",
    description: "Stored snapshots",
  },
  {
    name: "Activity",
    href: "/activity",
    icon: Activity,
    section: "Vault",
    description: "Audit log",
  },
  {
    name: "API guide",
    href: "/guide",
    icon: BookOpen,
    section: "Account",
    description: "Send backups from any app",
  },
  { name: "Settings", href: "/settings", icon: Settings, section: "Account" },
];
