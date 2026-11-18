// Copy buttons for the deploy snippets. No network, no dependencies.
(() => {
  const labels = {
    en: { copy: "Copy", copied: "Copied" },
    "zh-Hant": { copy: "複製", copied: "已複製" },
  };
  const lang = document.documentElement.lang || "en";
  const text = labels[lang] || labels.en;

  document.querySelectorAll("[data-copy]").forEach((button) => {
    button.textContent = text.copy;
    button.addEventListener("click", async () => {
      const code = button.parentElement.querySelector("pre code");
      if (!code) return;
      try {
        await navigator.clipboard.writeText(code.textContent);
        button.textContent = text.copied;
        button.classList.add("ok");
        window.setTimeout(() => {
          button.textContent = text.copy;
          button.classList.remove("ok");
        }, 1500);
      } catch {
        // Clipboard permission denied: select the block so the user can copy it.
        const range = document.createRange();
        range.selectNodeContents(code);
        const selection = window.getSelection();
        if (selection) {
          selection.removeAllRanges();
          selection.addRange(range);
        }
      }
    });
  });
})();
