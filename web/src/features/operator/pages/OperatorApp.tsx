// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Navigate, Route, Routes, useLocation, useNavigate } from "react-router";
import { ApiError, onUnauthorized } from "@/shared/api";
import "../texts";
import { useOperatorSession } from "../api";
import { OperatorShell } from "../components/OperatorShell";
import AccountPage from "./AccountPage";
import AuditPage from "./AuditPage";
import CustomerPage from "./CustomerPage";
import CustomersPage from "./CustomersPage";
import OperatorLoginPage from "./OperatorLoginPage";
import PlansPage from "./PlansPage";
import SystemPage from "./SystemPage";

/** The operator console on admin.<domain> (round 51): a chunk of its own, never loaded by a workspace's app. */
export default function OperatorApp() {
  return (
    <Routes>
      <Route path="login" element={<OperatorLoginPage />} />
      <Route
        element={
          <RequireOperator>
            <OperatorShell />
          </RequireOperator>
        }
      >
        <Route index element={<CustomersPage />} />
        <Route path="customers/:id" element={<CustomerPage />} />
        <Route path="plans" element={<PlansPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="system" element={<SystemPage />} />
        <Route path="account" element={<AccountPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}

function RequireOperator({ children }: { children: ReactNode }) {
  const session = useOperatorSession();
  const location = useLocation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  useEffect(() => {
    // A session that ended (8 hours, 30 idle minutes) sends the operator back to the login.
    onUnauthorized(() => {
      qc.clear();
      navigate("/login", { replace: true });
    });
    return () => onUnauthorized(() => {});
  }, [navigate, qc]);
  if (session.isPending) return null;
  if (session.error) {
    if (session.error instanceof ApiError && session.error.status === 401) {
      return <Navigate to="/login" replace state={{ from: location.pathname }} />;
    }
    return <p className="p-4 text-danger">{session.error.message}</p>;
  }
  return <>{children}</>;
}
