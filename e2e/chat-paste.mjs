import assert from "node:assert/strict";
import { chromium, request } from "playwright";
import { en, ko } from "../web/src/i18n/messages/interface.ts";
import { interface as zh } from "../web/src/i18n/messages/zh-Hans.ts";

const labels = { en, ko, "zh-Hans": zh };

async function pasteFiles(input, names, text = "") {
  await input.evaluate((element, { names, text }) => {
    const clipboardData = new DataTransfer();
    for (const name of names) clipboardData.items.add(new File([`document ${name}`], name, { type: "application/octet-stream" }));
    if (text) clipboardData.setData("text/plain", text);
    element.dispatchEvent(new ClipboardEvent("paste", { bubbles: true, cancelable: true, clipboardData }));
  }, { names, text });
}

async function pasteText(page, input, text) {
  await page.evaluate((value) => navigator.clipboard.writeText(value), text);
  await input.focus();
  await input.press("Control+V");
}

export async function runChatPasteChecks({ base, token, step, check, openPage, mutation = false }) {
  const api = await request.newContext({ baseURL: base, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });
  const createdTask = await api.post("/api/tasks", { data: { description: "Clipboard document fixture", goal: "Keep fixture idle", seed_first_intent: false } });
  check(createdTask.ok(), `task fixture ${createdTask.status()}`);
  const task = await createdTask.json();
  for (const locale of ["en", "ko", "zh-Hans"]) {
    for (const width of [1440, 390]) {
      for (const surface of ["standalone", "task"]) {
        if (process.env.PASTE_CASE && `${locale}/${width}/${surface}` !== process.env.PASTE_CASE) continue;
        const p = await openPage(locale, token);
        await p.setViewportSize({ width, height: 900 });
        await p.context().grantPermissions(["clipboard-read", "clipboard-write"]);
        const messages = [];
        let owners = 0;
        p.on("request", (r) => {
          if (r.method() !== "POST") return;
          const path = new URL(r.url()).pathname;
          if (path === "/api/conversations") owners++;
          if (/\/conversations\/\d+\/messages$/.test(path) || path === "/api/chat" || path.includes("/controls/inputs")) messages.push(path);
        });
        await step(`[${locale}/${width}/${surface}] document paste retains files and editable text`, async () => {
          await p.goto(surface === "standalone" ? `${base}/chat` : `${base}/function/tasks/detail?id=${task.id}`);
          const input = p.locator("textarea[data-chat-draft]").first();
          await input.waitFor();
          await p.waitForFunction(() => { const input = document.querySelector("textarea[data-chat-draft]"); return input && !input.disabled; });
          if (mutation) {
            // Temporarily omit only the application's paste wiring. This mutation
            // reproduces the previous file-picker-only composer without mocking uploads.
            await input.evaluate((element) => {
              const key = Object.keys(element).find((name) => name.startsWith("__reactProps"));
              if (key) element[key].onPaste = undefined;
            });
          }
          await pasteFiles(input, ["one.pdf"]);
          await pasteFiles(input, ["two.docx"], "Please review");
          await p.locator("[data-attachment-id]").nth(1).waitFor({ timeout: 5000 });
          check(await input.inputValue() === "Please review", "independent clipboard text lost");
          await p.waitForFunction(() => !document.querySelector("[data-attachment-id] .animate-spin"));
          if (surface === "standalone") check(owners === 1, `rapid paste created ${owners} owners`);
          while (await p.locator("[data-attachment-id]").count()) await p.locator("[data-attachment-id] button[aria-label]").first().click();
          await input.fill("");
          await pasteText(p, input, "x".repeat(3000));
          check((await input.inputValue()).length === 3000, "short paste left native text path");
          await input.press("Control+Z");
          check(await input.inputValue() === "", "native paste undo failed");
          const long = "  한글\n中文\n" + "z".repeat(3001);
          await pasteText(p, input, long);
          await p.getByRole("button", { name: labels[locale].chatPastedContent, exact: true }).click();
          check(await p.getByRole("dialog").locator("pre").textContent() === long, "paste preview changed content");
          await p.getByRole("dialog").press("Escape");
          await input.fill("later edits");
          await p.getByRole("button", { name: labels[locale].chatPastedContent, exact: true }).click();
          await p.getByRole("button", { name: labels[locale].chatInsertAsText, exact: true }).click();
          check(await input.inputValue() === `later edits${long}`, "restore overwrote later edits");
          let fail = true;
          await p.route("**/api/chat/upload?**", (route) => fail ? route.fulfill({ status: 503, body: "fixture upload offline" }) : route.continue());
          await pasteFiles(input, ["retry.pdf"]);
          await p.getByRole("button", { name: labels[locale].chatUploadRetry, exact: true }).waitFor();
          fail = false;
          await p.getByRole("button", { name: labels[locale].chatUploadRetry, exact: true }).click();
          await p.waitForFunction(() => !document.querySelector("[data-attachment-id] .animate-spin"));
          check(await p.getByRole("button", { name: labels[locale].chatUploadRetry, exact: true }).count() === 0, "retry did not finish");
          check(await input.inputValue() === `later edits${long}`, "failed upload changed draft");
          for (const mode of ["enter", "ctrl-enter"]) {
            await p.evaluate((value) => { localStorage.setItem("artex_chat_send_mode", value); window.dispatchEvent(new Event("storage")); }, mode);
            await input.fill("@");
          await input.press("End");
          await input.press("ArrowLeft");
          await input.press("ArrowRight");
          await p.getByRole("listbox").waitFor();
          await input.press("ArrowDown");
          await input.press("Enter");
          await input.evaluate((element) => {
            element.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
            element.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", isComposing: true, bubbles: true }));
            element.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true }));
          });
            check(messages.length === 0, "paste or IME unexpectedly sent a message");
          }
          // Resolve a real upload after a client-side route switch. The original
          // draft must own the result when the composer is mounted again.
          let release;
          const barrier = new Promise((resolve) => { release = resolve; });
          let began;
          const started = new Promise((resolve) => { began = resolve; });
          await p.route("**/api/chat/upload?**", async (route) => { began(); await barrier; await route.continue(); });
          await input.fill("origin draft");
          const key = await input.getAttribute("data-chat-draft");
          await p.keyboard.press("Escape");
          await pasteFiles(input, ["delayed.pdf"]);
          await Promise.race([started, new Promise((_, reject) => setTimeout(() => reject(new Error("delayed upload never started")), 5000))]);
          if (width === 390) await p.locator('[data-sidebar="trigger"]').first().click();
          const link = p.locator('a[href="/system/agents/"]').first();
          await link.evaluate((element) => element.click());
          await p.waitForURL("**/system/agents/**");
          await input.waitFor({ state: "detached" });
          release();
          await p.goBack();
          await p.locator(`textarea[data-chat-draft="${key}"]`).waitFor();
          try { await p.getByRole("button", { name: /^delayed(?:-\d+)?\.pdf$/ }).waitFor({timeout:5000}); }
          catch (error) { await p.screenshot({path:".superpowers/paste-failure.png"}); console.error(await p.locator("textarea[data-chat-draft], [data-attachment-id]").evaluateAll(nodes=>nodes.map(n=>({key:n.dataset.chatDraft,text:n.textContent,value:n.value})))); throw error; }
          await p.waitForFunction(() => !document.querySelector("[data-attachment-id] .animate-spin"));
          await p.waitForFunction(() => document.querySelector("textarea[data-chat-draft]")?.value === "origin draft").catch(async error => { console.error("restored draft", await input.inputValue()); throw error; });
          check(await input.inputValue() === "origin draft", "late upload changed originating draft");
        });
        await p.context().close();
      }
    }
  }
  await api.dispose();
}

if (process.argv[1]?.endsWith("chat-paste.mjs")) {
  const base = process.env.SCOPEWEAVER_URL ?? "http://127.0.0.1:18787";
  const dsn = new URL((process.env.ARTEX_PG_DSN ?? "").replace("@/", "@localhost/"));
  assert.ok(dsn.pathname.slice(1).startsWith("scopeweaver_test_"), "requires disposable database");
  const api = await request.newContext({ baseURL: base });
  await api.post("/api/auth/init", { data: { password: "chat-test-pass-1234" } });
  const login = await api.post("/api/auth/login", { data: { username: "ARTEX", password: "chat-test-pass-1234" } });
  assert.ok(login.ok(), `login ${login.status()}`);
  const { token } = await login.json();
  const browser = await chromium.launch();
  let failures = 0;
  try {
    await runChatPasteChecks({ base, token, mutation: process.env.PASTE_MUTATION === "1", check: assert.ok,
      step: async (name, fn) => { try { await fn(); console.log(`PASS ${name}`); } catch (error) { failures++; console.error(`FAIL ${name}: ${error.stack}`); if (process.env.PASTE_MUTATION === "1") throw error; } },
      openPage: async (locale, jwt) => { const context = await browser.newContext(); await context.addInitScript(([l, t]) => { localStorage.setItem("scopeweaver.locale", l); localStorage.setItem("artex_token", t); }, [locale, jwt]); return context.newPage(); },
    });
  } finally { await browser.close(); await api.dispose(); }
  if (failures) process.exitCode = 1;
}
