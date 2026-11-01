import type { Schemas } from "@/shared/api";
import type { Actor, HostAgent } from "./directory";

// The "Új agent" dialog's model (docs/handoff/33, part 4). An agent is
// configured once: everything here goes to POST /agents in one call.

export const steps = ["profile", "machine", "access", "instructions", "limits", "summary"] as const;
export type Step = (typeof steps)[number];
export type GrantAccess = "read" | "write";

export type Draft = {
  handle: string;
  displayName: string;
  avatarEmoji: string;
  /** "" = the machine's colour. */
  color: string;
  description: string;
  apiKey: boolean;
  hostId: string;
  client: "claude-code" | "antigravity";
  model: string;
  workModel: string;
  mode: "direct" | "compact";
  allowWork: boolean;
  permissionMode: string;
  sandbox: boolean;
  rootDir: string;
  worktreeRepo: string;
  grants: Map<string, GrantAccess>;
  approvals: "none" | "chat";
  allowPreview: boolean;
  allowWebRead: boolean;
  instructions: string;
  /** 0 = no limit. */
  maxSessionTokens: number;
  dailyTokenLimit: number;
  channelIds: string[];
};

export function emptyDraft(hostId = ""): Draft {
  return {
    handle: "", displayName: "", avatarEmoji: "", color: "", description: "", apiKey: false, hostId,
    client: "claude-code", model: "", workModel: "", mode: "compact", allowWork: true, permissionMode: "auto",
    sandbox: true, rootDir: "", worktreeRepo: "", grants: new Map(), approvals: "none", allowPreview: false, allowWebRead: false,
    instructions: "", maxSessionTokens: 150000, dailyTokenLimit: 0, channelIds: [],
  };
}

/** Permission modes a client offers; the first is the default. Never a skip-permissions default. */
export const permissionModes: Record<Draft["client"], string[]> = {
  "claude-code": ["auto", "default", "acceptEdits", "plan", "bypassPermissions"],
  antigravity: ["auto", "default", "accept-edits"],
};
export const riskyModes = new Set(["bypassPermissions"]);
export const knownModels: Record<Draft["client"], string[]> = {
  "claude-code": ["opus", "sonnet", "haiku", "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"],
  antigravity: ["gemini-3-pro", "gemini-3-flash", "claude-opus-4-6-thinking"],
};
export const sessionPresets = [100000, 150000, 300000, 0];

const handlePattern = /^[a-z0-9][a-z0-9-]{1,31}$/;

// Mirrors the server's check (agents.LooksSecret), so the editor can warn early.
const secret =
  /(col_|colh_|sk-ant-[a-z0-9]*-|sk-|AIza|ya29\.|ghp_|github_pat_|glpat-|xox[abp]-|AKIA)[A-Za-z0-9_\-.]{8,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|\b[A-Z0-9_]*(TOKEN|SECRET|PASSWORD|API_KEY)\s*[=:]\s*\S{6,}/i;
export const looksSecret = (text: string) => secret.test(text);
export const MaxInstructions = 8000;

export type HandleInfo = { active: Set<string>; archived: Set<string> };
export type Errors = Partial<Record<string, string>>;

/** The i18n keys of a step's problems ({} when it is fine). */
export function stepErrors(d: Draft, step: Step, handles: HandleInfo): Errors {
  const e: Errors = {};
  if (step === "profile") {
    if (!handlePattern.test(d.handle)) e.handle = "errors.handle";
    else if (handles.active.has(d.handle)) e.handle = "errors.handleTaken";
    if (d.description.length > 300) e.description = "errors.description";
  }
  if (step === "machine" && !d.apiKey) {
    if (!d.hostId) e.hostId = "errors.host";
    if (!d.client) e.client = "errors.client";
  }
  if (step === "access" && !d.apiKey) {
    if ((d.mode === "direct" || !d.sandbox) && !d.rootDir && !d.worktreeRepo) e.rootDir = "errors.rootDir";
    if (d.allowPreview && (d.mode !== "direct" || d.sandbox)) e.allowPreview = "errors.preview";
    if (d.allowWebRead && !d.sandbox) e.allowWebRead = "errors.webRead";
    if (d.worktreeRepo && d.mode === "direct" && d.sandbox) e.worktreeRepo = "errors.worktree";
    if (d.approvals === "chat" && d.client !== "claude-code") e.approvals = "errors.approvals";
  }
  if (step === "instructions") {
    if ([...d.instructions].length > MaxInstructions) e.instructions = "errors.instructionsLong";
    else if (looksSecret(d.instructions)) e.instructions = "errors.secret";
  }
  if (step === "limits" && (d.maxSessionTokens < 0 || d.dailyTokenLimit < 0)) e.limits = "errors.limits";
  return e;
}

export function firstInvalidStep(d: Draft, handles: HandleInfo): Step | null {
  return steps.find((s) => Object.keys(stepErrors(d, s, handles)).length > 0) ?? null;
}

export function toRequest(d: Draft): Schemas["CreateAgentRequest"] {
  const req: Schemas["CreateAgentRequest"] = {
    handle: d.handle,
    ...(d.displayName.trim() ? { display_name: d.displayName.trim() } : {}),
    ...(d.avatarEmoji ? { avatar_emoji: d.avatarEmoji } : {}),
    ...(d.color ? { color: d.color as Schemas["Hue"] } : {}),
    ...(d.description.trim() ? { description: d.description.trim() } : {}),
    ...(d.instructions.trim() ? { instructions: d.instructions.trim() } : {}),
    ...(d.channelIds.length ? { channel_ids: d.channelIds } : {}),
  };
  if (d.apiKey) return { ...req, api_key: true };
  return {
    ...req,
    host: {
      host_id: d.hostId,
      client: d.client,
      ...(d.model.trim() ? { model: d.model.trim() } : {}),
      ...(d.workModel.trim() ? { work_model: d.workModel.trim() } : {}),
      allow_work: d.allowWork,
      permission_mode: d.permissionMode,
      mode: d.mode,
      sandbox: d.sandbox,
      ...(d.rootDir ? { root_dir: d.rootDir } : {}),
      ...(d.worktreeRepo ? { worktree_repo: d.worktreeRepo } : {}),
      approvals: d.approvals,
      allow_preview: d.allowPreview,
      allow_web_read: d.sandbox && d.allowWebRead,
      max_session_tokens: d.maxSessionTokens,
      daily_token_limit: d.dailyTokenLimit,
    },
    ...(d.grants.size ? { grants: [...d.grants].map(([path, mode]) => ({ path, mode })) } : {}),
  };
}

/** "Másolat új agentként": every setting of an agent, archived ones too. */
export function fromAgent(actor: Actor, config: HostAgent | undefined, instructions: string): Draft {
  const d = emptyDraft(config?.host_id ?? "");
  return {
    ...d,
    handle: actor.handle, displayName: actor.display_name, avatarEmoji: actor.avatar_emoji ?? "", color: actor.color ?? "",
    description: actor.description ?? "", apiKey: !config, instructions,
    ...(config
      ? {
          client: config.client as Draft["client"], model: config.model, workModel: config.work_model === config.model ? "" : config.work_model,
          mode: config.mode as Draft["mode"], allowWork: config.allow_work, permissionMode: config.permission_mode, sandbox: config.sandbox,
          rootDir: config.worktree_repo && config.root_dir === config.worktree_repo ? "" : config.root_dir, worktreeRepo: config.worktree_repo,
          grants: new Map(config.grants.map((g) => [g.path, g.mode as GrantAccess])), approvals: config.approvals as Draft["approvals"],
          allowPreview: Boolean(config.allow_preview), allowWebRead: Boolean(config.allow_web_read),
          maxSessionTokens: config.max_session_tokens, dailyTokenLimit: config.daily_token_limit,
        }
      : {}),
  };
}

/** Which step a server error belongs to, from its message. */
export function stepForError(detail: string): Step {
  const m = detail.toLowerCase();
  if (/handle|avatar|colou?r|description/.test(m)) return "profile";
  if (/instruction|secret|prompt/.test(m)) return "instructions";
  if (/channel|limit|max_session|daily/.test(m)) return "limits";
  if (/path|root|grant|folder|worktree|preview|web_read|approvals|sandbox/.test(m)) return "access";
  return "machine";
}
