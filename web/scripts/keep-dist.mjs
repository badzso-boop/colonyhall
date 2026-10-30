// Vite empties dist/ on every build; recreate the tracked placeholder so
// `go build` keeps working and git does not see it as deleted.
import { writeFileSync } from "node:fs";

writeFileSync(new URL("../dist/.keep", import.meta.url), "");
