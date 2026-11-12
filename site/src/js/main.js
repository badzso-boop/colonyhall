// Entry point (an ES module, so deferred). The page is complete without it;
// this adds the copy buttons and, in later steps, the colony.
import { t } from "./i18n.js";

for (const el of document.querySelectorAll("[data-js-only]")) el.hidden = false;

// Copy buttons next to commands.
for (const btn of document.querySelectorAll("[data-copy]")) {
  btn.addEventListener("click", async () => {
    const code = btn.closest("[data-copy-root], .cmd")?.querySelector("[data-copy-text]");
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code.textContent.trim());
      btn.textContent = t("copy.done");
    } catch {
      const range = document.createRange();
      range.selectNodeContents(code);
      getSelection().removeAllRanges();
      getSelection().addRange(range);
      btn.textContent = t("copy.failed");
    }
    setTimeout(() => { btn.textContent = t("copy.button"); }, 1800);
  });
}
