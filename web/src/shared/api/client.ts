// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// The typed REST client: paths, parameters and bodies come from the
// generated schema.d.ts (api/openapi.yaml → `make gen`), never by hand.
// The browser sends the session cookie; scripts use Bearer tokens.
let csrfToken = "";
let unauthorized: () => void = () => {};

/** On these a 401 is a wrong password, which the form shows; the session is fine. */
const passwordChecked = (path: string) => path.endsWith("/auth/login") || path.endsWith("/admin/emergency-clear") || path.endsWith("/password");

/** The CSRF token of the cookie session (from /auth/session or /auth/login). */
export function setCsrfToken(token: string): void {
  csrfToken = token;
}

export function getCsrfToken(): string {
  return csrfToken;
}

/** Called on any 401 (the session expired): the app sends the user to /login. */
export function onUnauthorized(handler: () => void): void {
  unauthorized = handler;
}

export function createApi(baseUrl = `${globalThis.location?.origin ?? ""}/api/v1`) {
  // fetch is looked up on each call (not captured here), so tests can stub it.
  const client = createClient<paths>({ baseUrl, credentials: "same-origin", fetch: (req) => globalThis.fetch(req) });
  client.use({
    onRequest({ request }) {
      // Browser requests that change state carry the session's CSRF token.
      if (request.method !== "GET" && request.method !== "HEAD" && csrfToken) {
        request.headers.set("X-CSRF-Token", csrfToken);
      }
      return request;
    },
    onResponse({ response, request }) {
      if (response.status === 401 && !passwordChecked(new URL(request.url).pathname)) unauthorized();
      return response;
    },
  });
  return client;
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
