import { AdminAuthProvider } from "@/components/admin/auth/admin-auth-provider";
import { AdminSidebarProvider } from "@/context/admin-sidebar-context";
import { Outlet } from "react-router-dom";

export function AdminRootLayout() {
  return (
    <div className="admin-shell min-h-dvh w-full">
      <AdminAuthProvider>
        <AdminSidebarProvider>
          <Outlet />
        </AdminSidebarProvider>
      </AdminAuthProvider>
    </div>
  );
}
