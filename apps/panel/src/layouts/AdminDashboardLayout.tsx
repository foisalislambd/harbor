import { AdminGuard } from "@/components/admin/auth/admin-guard";
import { AdminBackdrop } from "@/components/admin/layout/admin-backdrop";
import { AdminHeader } from "@/components/admin/layout/admin-header";
import { AdminSidebar } from "@/components/admin/layout/admin-sidebar";
import { Outlet } from "react-router-dom";

export function AdminDashboardLayout() {
  return (
    <AdminGuard>
      <div className="admin-shell admin-main flex h-dvh w-full overflow-hidden">
        <AdminSidebar />
        <AdminBackdrop />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <AdminHeader />
          <main className="admin-scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden">
            <div className="w-full px-6 py-6">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </AdminGuard>
  );
}
