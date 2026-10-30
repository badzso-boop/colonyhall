import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// The typed REST client: paths, parameters and bodies come from the
// generated schema.d.ts (api/openapi.yaml → `make gen`), never by hand.
// The browser sends the session cookie; scripts use Bearer tokens.
export function createApi(baseUrl = "/api/v1") {
  return createClient<paths>({ baseUrl, credentials: "same-origin" });
}

export const api = createApi();

/** All schemas of the contract, e.g. `Schemas["Message"]`. */
export type Schemas = components["schemas"];

/** RFC 9457 problem returned by the server on errors. */
export type Problem = { type?: string; title?: string; status?: number; detail?: string };

/** Thrown by `unwrap` when the server answers with an error. */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem | undefined;

  constructor(status: number, problem: Problem | undefined) {
    super(problem?.detail || problem?.title || `HTTP ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }
}

/**
 * unwrap turns an openapi-fetch result into its data or an ApiError, so
 * TanStack Query sees a rejected promise on failure:
 *   queryFn: async () => unwrap(await api.GET("/me"))
 */
export function unwrap<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.error !== undefined || !result.response.ok) {
    throw new ApiError(result.response.status, (result.error ?? undefined) as Problem | undefined);
  }
  return result.data as T;
}
