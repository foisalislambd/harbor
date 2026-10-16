import { AdminGuard } from "@/components/admin/auth/admin-guard";
import { AdminBackdrop } from "@/components/admin/layout/admin-backdrop";
import { AdminHeader } from "@/components/admin/layout/admin-header";
import { AdminSidebar } from "@/components/admin/layout/admin-sidebar";
import {
  SIDEBAR_WIDTH_COLLAPSED,
  SIDEBAR_WIDTH_EXPANDED,
  useAdminSidebar,
} from "@/context/admin-sidebar-context";
import { Outlet } from "react-router-dom";

export function AdminDashboardLayout() {
  const { isExpanded, isDesktop } = useAdminSidebar();
  const sidebarWidth = isDesktop ? (isExpanded ? SIDEBAR_WIDTH_EXPANDED : SIDEBAR_WIDTH_COLLAPSED) : 0;

  return (
    <AdminGuard>
      <div className="admin-shell admin-main flex h-dvh w-full overflow-hidden">
        <div
          className="hidden shrink-0 transition-[width] duration-300 ease-in-out lg:block"
          style={{ width: sidebarWidth }}
          aria-hidden
        />
        <AdminSidebar />
        <AdminBackdrop />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <AdminHeader />
          <main className="admin-scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden">
            <div className="w-full px-3 py-4 sm:px-4 sm:py-5">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </AdminGuard>
  );
}
