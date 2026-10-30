import { Suspense, useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createQueryClient } from "@/shared/api";
import { AppRoutes } from "./routes";

export function App() {
  const [queryClient] = useState(createQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={null}>
        <AppRoutes />
      </Suspense>
    </QueryClientProvider>
  );
}
