// @vitest-environment node
// The layer rules of docs/08-frontend-architektura.md (2., 10., 11.) are
// enforced by eslint.config.js; this test makes sure they keep catching
// violations, so a config change cannot silently switch them off.
import { ESLint } from "eslint";

const eslint = new ESLint({ cwd: new URL("../..", import.meta.url).pathname });

async function rules(file: string, code: string): Promise<string[]> {
  const [result] = await eslint.lintText(code, { filePath: file });
  return (result?.messages ?? []).map((m) => m.ruleId ?? m.message);
}

// The first lint loads the config, the plugins and the TypeScript resolver,
// which can take long while the other test files run in parallel.
beforeAll(async () => {
  await rules("src/shared/lib/warmup.ts", "export const w = 1;");
}, 180_000);

const cases: Array<[string, string, string, string | null]> = [
  ["feature → another feature's internals", "src/features/demo/x.ts", 'import P from "@/features/design/pages/DesignPage";\nexport const a = P;', "boundaries/dependencies"],
  ["feature → another feature's index", "src/features/demo/x.ts", 'import { DesignPage } from "@/features/design";\nexport const a = DesignPage;', null],
  ["feature → shared", "src/features/demo/x.ts", 'import { cn } from "@/shared/lib";\nexport const a = cn;', null],
  ["feature → app", "src/features/demo/x.ts", 'import { App } from "@/app/App";\nexport const a = App;', "boundaries/dependencies"],
  ["shared → feature", "src/shared/lib/x.ts", 'import { DesignPage } from "@/features/design";\nexport const a = DesignPage;', "boundaries/dependencies"],
  ["layout → feature internals", "src/layout/x.ts", 'import P from "@/features/design/pages/DesignPage";\nexport const a = P;', "boundaries/dependencies"],
  ["layout → feature index", "src/layout/x.ts", 'import { DesignPage } from "@/features/design";\nexport const a = DesignPage;', null],
  ["app → feature index", "src/app/x.ts", 'import { DesignPage } from "@/features/design";\nexport const a = DesignPage;', null],
  ["../../ import", "src/features/demo/sub/x.ts", 'import { cn } from "../../../shared/lib";\nexport const a = cn;', "no-restricted-imports"],
  ["dangerouslySetInnerHTML", "src/features/demo/X.tsx", 'export const X = () => <div dangerouslySetInnerHTML={{ __html: "x" }} />;', "no-restricted-syntax"],
  ["innerHTML", "src/features/demo/x.ts", 'export const f = (e: HTMLElement) => { e.innerHTML = "x"; };', "no-restricted-syntax"],
  ["a file outside every layer", "src/stray.ts", "export const s = 1;", "boundaries/no-unknown-files"],
];

test.each(cases)("%s", async (_name, file, code, want) => {
  const got = await rules(file, code);
  if (want) expect(got).toContain(want);
  else expect(got).toEqual([]);
}, 60_000);
