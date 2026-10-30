import { useMutation } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

/** Creates a schedule (the composer's /schedule, /daily, /cron; the builder in round 32). */
export function useCreateSchedule() {
  return useMutation({
    mutationFn: async (body: Schemas["CreateScheduleRequest"]) => unwrap(await api.POST("/schedules", { body })),
  });
}
