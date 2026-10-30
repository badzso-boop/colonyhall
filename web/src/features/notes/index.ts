import { lazy } from "react";

export { NotesWindow } from "./components/NotesWindow";
export { useNotesBadge } from "./badges";
export const NotesPage = lazy(() => import("./pages/NotesPage"));
