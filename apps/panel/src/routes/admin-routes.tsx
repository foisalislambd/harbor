import { AdminAuthLayout } from "@/layouts/AdminAuthLayout";
import { AdminDashboardLayout } from "@/layouts/AdminDashboardLayout";
import { AdminRootLayout } from "@/layouts/AdminRootLayout";
import ActivityPage from "@/pages/admin/ActivityPage";
import BackupsPage from "@/pages/admin/BackupsPage";
import DashboardPage from "@/pages/admin/DashboardPage";
import GuidePage from "@/pages/admin/GuidePage";
import LoginPage from "@/pages/admin/LoginPage";
import ProjectDetailPage from "@/pages/admin/ProjectDetailPage";
import ProjectsPage from "@/pages/admin/ProjectsPage";
import SettingsPage from "@/pages/admin/SettingsPage";
import { Route } from "react-router-dom";

export const zenPanelAdminRoute = (
  <Route path="/" element={<AdminRootLayout />}>
    <Route element={<AdminAuthLayout />}>
      <Route path="login" element={<LoginPage />} />
    </Route>
    <Route element={<AdminDashboardLayout />}>
      <Route index element={<DashboardPage />} />
      <Route path="projects" element={<ProjectsPage />} />
      <Route path="projects/:projectId" element={<ProjectDetailPage />} />
      <Route path="backups" element={<BackupsPage />} />
      <Route path="activity" element={<ActivityPage />} />
      <Route path="guide" element={<GuidePage />} />
      <Route path="settings" element={<SettingsPage />} />
    </Route>
  </Route>
);
