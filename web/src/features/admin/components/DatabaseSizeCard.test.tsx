// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { render, screen } from "@testing-library/react";
import type { SystemStatus } from "../model";
import { DatabaseSizeCard } from "../components/DatabaseSizeCard";

type Database = SystemStatus["database"];
const MB = 1024 * 1024;

// Round 50: the database card on the System status page.
test("the database card shows the total, the largest tables and the growth", () => {
  const db: Database = {
    ok: true,
    latency_ms: 0.4,
    size_bytes: 21.4 * MB,
    tables: [
      { name: "message", total_bytes: 8 * MB, rows_estimate: 12345 },
      { name: "api_token", total_bytes: 1536, rows_estimate: 20 },
    ],
    growth: [
      { date: "2026-09-01", size_bytes: 10 * MB }, // older than 30 days: left out
      { date: "2026-10-12", size_bytes: 18.2 * MB },
      { date: "2026-10-30", size_bytes: 20 * MB },
      { date: "2026-11-11", size_bytes: 21.4 * MB },
    ],
  };
  render(<DatabaseSizeCard database={db} />);
  screen.getByText("Adatbázis mérete");
  expect(screen.getAllByText("21,4 MB").length).toBeGreaterThan(0);
  screen.getByText("message");
  screen.getByText("8 MB");
  screen.getByText("1,5 kB");
  screen.getByText(/12.345/);
  screen.getByText("+3,2 MB 30 nap alatt");
  screen.getByRole("img", { name: /^Az elmúlt 30 napban: 18,2 MB, 20 MB, 21,4 MB$/ });
});

test("the database card says when there are too few samples", () => {
  render(<DatabaseSizeCard database={{ ok: true, latency_ms: 1, size_bytes: 2048, tables: [], growth: [{ date: "2026-11-11", size_bytes: 2048 }] }} />);
  screen.getByText("2 kB");
  screen.getByText("még nincs elég minta");
});

test("the memory store has no database", () => {
  render(<DatabaseSizeCard database={{ ok: true, latency_ms: 0 }} />);
  screen.getByText("Memória-tár: nincs adatbázis.");
  expect(screen.queryByText("Teljes méret")).toBeNull();
});
