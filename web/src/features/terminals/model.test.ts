import { formatDuration, sshAttach, terminalBadge } from "./model";

const term = (status: "running" | "finished" | "killed" | "error", exit_code: number | null = null) => ({
  name: "build", status, command: "go test ./...", cwd: "/", channel_id: "c", started_at: "", duration_ms: 0, exit_code,
});

test("terminalBadge follows the status and the exit code", () => {
  expect(terminalBadge(term("running"))).toMatchObject({ key: "status.running", tone: "warning" });
  expect(terminalBadge(term("finished", 0))).toMatchObject({ key: "status.finished", code: 0, tone: "success" });
  expect(terminalBadge(term("finished", 2))).toMatchObject({ key: "status.error", code: 2, tone: "danger" });
  expect(terminalBadge(term("killed"))).toMatchObject({ key: "status.killed", tone: "neutral" });
});

test("formatDuration and the SSH line", () => {
  expect(formatDuration(2_592_000)).toBe("43:12");
  expect(formatDuration(3_725_000)).toBe("1:02:05");
  expect(sshAttach("backend-claude", "build")).toBe("tmux -L colonyhall attach -r -t colony-backend-claude-build");
});
