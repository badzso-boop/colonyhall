// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken, unwrap, type Schemas } from "@/shared/api";

export type Actor = Schemas["Actor"];
export type Session = Schemas["SessionInfo"];

export const identityKeys = {
  session: ["identity", "session"] as const,
  version: ["identity", "version"] as const,
  actors: ["identity", "actors"] as const,
};

/** The signed-in actor and the CSRF token of the cookie session. */
export function useSession() {
  return useQuery({
    queryKey: identityKeys.session,
    queryFn: async () => {
      const s = unwrap(await api.GET("/auth/session"));
      setCsrfToken(s.csrf_token);
      return s;
    },
    retry: false,
    staleTime: Infinity,
  });
}

/** The server's version, licence and source link (AGPL section 13); public. */
export function useVersion() {
  return useQuery({
    queryKey: identityKeys.version,
    queryFn: async () => unwrap(await api.GET("/version")),
    staleTime: Infinity,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["LoginRequest"]) => unwrap(await api.POST("/auth/login", { body })),
    onSuccess: (s) => {
      setCsrfToken(s.csrf_token);
      qc.setQueryData(identityKeys.session, s);
    },
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      await api.POST("/auth/logout");
    },
    onSettled: () => {
      setCsrfToken("");
      qc.clear();
    },
  });
}

/**
 * Edits a profile (one's own; admins anyone's). The display name is not
 * part of the API. The result replaces the actor in the cached list and,
 * for one's own profile, in the session.
 */
export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ actorId, body }: { actorId: string; body: Schemas["UpdateActorProfileRequest"] }) =>
      unwrap(await api.PATCH("/actors/{actorId}/profile", { params: { path: { actorId } }, body })),
    onSuccess: (a) => {
      qc.setQueryData<Map<string, Actor>>(identityKeys.actors, (m) => (m ? new Map(m).set(a.id, a) : m));
      qc.setQueryData<Session>(identityKeys.session, (s) => (s && s.actor.id === a.id ? { ...s, actor: { ...s.actor, ...a } } : s));
    },
  });
}

/** Every actor of the workspace, by id. */
export function useActors() {
  return useQuery({
    queryKey: identityKeys.actors,
    queryFn: async () => {
      const { actors } = unwrap(await api.GET("/actors"));
      return new Map(actors.map((a) => [a.id, a]));
    },
  });
}

/** Round 51: the workspace an owner invite is for (public; the token travels in the body). */
export function useInviteLookup(token: string) {
  return useQuery({
    queryKey: ["identity", "invite", token],
    queryFn: async () => unwrap(await api.POST("/invites/lookup", { body: { token } })),
    enabled: token !== "",
    retry: false,
    staleTime: Infinity,
  });
}

/** Round 51: become the owner with the invite, and be logged in. */
export function useAcceptInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["AcceptInviteRequest"]) => unwrap(await api.POST("/invites/accept", { body })),
    onSuccess: (s) => {
      setCsrfToken(s.csrf_token);
      qc.setQueryData(identityKeys.session, s);
    },
  });
}
