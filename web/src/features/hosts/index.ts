import { lazy } from "react";

export {
  dirsFailure,
  hostKeys,
  pickClaudeLimits,
  registerHostEvents,
  useClaudeLimits,
  useHostDirs,
  useHosts,
  type ClaudeLimits,
  type DirsFailure,
  type Host,
  type HostDirEntry,
} from "./api";
export { FolderPicker, type FolderPickerProps } from "./components/FolderPicker";
export { rootOf, type Access, type PickMode, type Selection } from "./picker";
export { isOnline } from "./model";
export const HostsPage = lazy(() => import("./pages/HostsPage"));
