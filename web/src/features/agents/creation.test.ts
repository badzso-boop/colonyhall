import { emptyDraft, firstInvalidStep, fromAgent, looksSecret, stepErrors, stepForError, toRequest } from "./creation";

const handles = { active: new Set(["backend"]), archived: new Set(["old"]) };

test("step checks: handle, machine, access rules, secrets", () => {
  const d = { ...emptyDraft("h1"), handle: "Bad Handle" };
  expect(stepErrors(d, "profile", handles)).toEqual({ handle: "errors.handle" });
  expect(stepErrors({ ...d, handle: "backend" }, "profile", handles)).toEqual({ handle: "errors.handleTaken" });
  expect(stepErrors({ ...d, handle: "old" }, "profile", handles)).toEqual({});
  expect(stepErrors({ ...emptyDraft(), handle: "x1" }, "machine", handles)).toEqual({ hostId: "errors.host" });
  expect(stepErrors({ ...emptyDraft(), apiKey: true }, "machine", handles)).toEqual({});
  const full = { ...emptyDraft("h1"), mode: "direct" as const, sandbox: false };
  expect(stepErrors(full, "access", handles)).toEqual({ rootDir: "errors.rootDir" });
  expect(stepErrors({ ...full, rootDir: "/app", allowWebRead: true }, "access", handles)).toEqual({ allowWebRead: "errors.webRead" });
  expect(stepErrors({ ...emptyDraft("h1"), allowPreview: true }, "access", handles)).toEqual({ allowPreview: "errors.preview" });
  expect(stepErrors({ ...emptyDraft("h1"), client: "antigravity", approvals: "chat" }, "access", handles)).toEqual({ approvals: "errors.approvals" });
  expect(stepErrors({ ...emptyDraft("h1"), instructions: "GITHUB_TOKEN=ghp_abcdefghijklmnop" }, "instructions", handles)).toEqual({ instructions: "errors.secret" });
  expect(looksSecret("Írj tömören, magyarul.")).toBe(false);
  expect(firstInvalidStep({ ...emptyDraft("h1"), handle: "ok-bot" }, handles)).toBeNull();
  expect(firstInvalidStep({ ...emptyDraft(""), handle: "ok-bot" }, handles)).toBe("machine");
});

test("the request: a machine agent with grants, or an API-key agent", () => {
  const d = { ...emptyDraft("h1"), handle: "ok-bot", color: "teal", model: "sonnet", grants: new Map([["/work/app", "write" as const]]), channelIds: ["c1"], instructions: " Légy tömör. " };
  expect(toRequest(d)).toEqual({
    handle: "ok-bot", color: "teal", instructions: "Légy tömör.", channel_ids: ["c1"],
    host: { host_id: "h1", client: "claude-code", model: "sonnet", allow_work: true, permission_mode: "auto", mode: "compact", sandbox: true,
      approvals: "none", allow_preview: false, allow_web_read: false, max_session_tokens: 150000, daily_token_limit: 0 },
    grants: [{ path: "/work/app", mode: "write" }],
  });
  expect(toRequest({ ...emptyDraft(), handle: "api-bot", apiKey: true })).toEqual({ handle: "api-bot", api_key: true });
});

test("a copy of an agent round-trips into the same request", () => {
  const actor = { id: "a", kind: "agent", handle: "ok-bot", display_name: "Ok", role: "member", state: "active", created_at: "", color: "pink" } as const;
  const config = { agent_id: "a", handle: "ok-bot", host_id: "h1", client: "claude-code", model: "opus", work_model: "opus", allow_work: true, permission_mode: "plan",
    mode: "direct", root_dir: "/repo", worktree_repo: "/repo", sandbox: false, approvals: "chat", max_session_tokens: 300000, daily_token_limit: 5,
    grants: [{ id: "g", path: "/data", mode: "read" }], allow_preview: true } as never;
  const req = toRequest(fromAgent(actor as never, config, "x"));
  expect(req.host).toMatchObject({ host_id: "h1", model: "opus", mode: "direct", sandbox: false, worktree_repo: "/repo", approvals: "chat", allow_preview: true, permission_mode: "plan", max_session_tokens: 300000 });
  expect(req.host).not.toHaveProperty("root_dir");
  expect(req.grants).toEqual([{ path: "/data", mode: "read" }]);
  expect(req.color).toBe("pink");
});

test("a server error goes to its step", () => {
  expect(stepForError("handle @x is taken")).toBe("profile");
  expect(stepForError("the path is outside the folders the machine reported")).toBe("access");
  expect(stepForError("the instructions look like they contain a secret")).toBe("instructions");
  expect(stepForError("a chosen channel does not exist")).toBe("limits");
  expect(stepForError("the machine has not reported this client")).toBe("machine");
});
