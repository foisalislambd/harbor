import { ThemeProvider } from "@/components/theme/theme-provider";
import { zenPanelAdminRoute } from "@/routes/admin-routes";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";

export default function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <Routes>
          {zenPanelAdminRoute}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </ThemeProvider>
  );
}
