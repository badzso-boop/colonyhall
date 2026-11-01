import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken, unwrap, type Schemas } from "@/shared/api";

export type Actor = Schemas["Actor"];
export type Session = Schemas["SessionInfo"];

export const identityKeys = {
  session: ["identity", "session"] as const,
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
