import { Route, Routes } from "react-router";
import { DesignPage } from "@/features/design";
import { ScaffoldPage } from "@/features/scaffold";

// Route → the page a feature exports from its index.ts (lazy loaded).
export function AppRoutes() {
  return (
    <Routes>
      <Route path="design" element={<DesignPage />} />
      <Route path="*" element={<ScaffoldPage />} />
    </Routes>
  );
}
