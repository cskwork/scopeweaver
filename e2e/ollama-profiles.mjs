// Native Ollama profile browser checks, called from smoke.mjs after login.
// A local fixture stands in for Ollama, so no real model or provider is contacted.
// The ScopeWeaver backend must reach the fixture; set E2E_OLLAMA_FIXTURE_HOST when
// the backend runs elsewhere (default 127.0.0.1, i.e. the binary runs on this host).
import http from "node:http";
import { en as iEn, ko as iKo } from "../web/src/i18n/messages/interface.ts";
import { en as pEn, ko as pKo } from "../web/src/i18n/messages/providers.ts";
import { en as sEn, ko as sKo } from "../web/src/i18n/messages/settings.ts";

const T = { en: { i: iEn, p: pEn, s: sEn }, ko: { i: iKo, p: pKo, s: sKo } };
const TOOL_MODEL = "qwen2.5-coder:32b";
const NO_TOOLS_MODEL = "fixture-notools:8b";
const MODELS = {
  [TOOL_MODEL]: { details: { parameter_size: "32.8B", quantization_level: "Q4_K_M" }, capabilities: ["completion", "tools"] },
  [NO_TOOLS_MODEL]: { details: { parameter_size: "8.0B", quantization_level: "Q4_0" }, capabilities: ["completion"] },
};

function startFixture() {
  const hits = [];
  const chats = [];
  let lastModel = "";
  const server = http.createServer((req, res) => {
    let raw = "";
    req.on("data", (c) => { raw += c; });
    req.on("end", () => {
      const body = raw ? JSON.parse(raw) : {};
      hits.push(`${req.method} ${req.url}`);
      const send = (code, value) => { res.writeHead(code, { "content-type": "application/json" }); res.end(JSON.stringify(value)); };
      if (req.method === "GET" && req.url === "/api/tags") return send(200, { models: Object.entries(MODELS).map(([name, m]) => ({ name, model: name, details: m.details })) });
      if (req.method === "POST" && req.url === "/api/show") {
        const m = MODELS[body.model];
        return m ? send(200, { capabilities: m.capabilities, details: m.details }) : send(404, { error: `model '${body.model}' not found` });
      }
      if (req.method === "POST" && req.url === "/api/chat") {
        chats.push(body);
        lastModel = body.model;
        if (!MODELS[body.model]) return send(404, { error: `model '${body.model}' not found` });
        const toolCalls = MODELS[body.model].capabilities.includes("tools") && body.tools?.length ? [{ function: { name: body.tools[0].function.name, arguments: { value: "OK" } } }] : [];
        return send(200, { model: body.model, message: { role: "assistant", content: toolCalls.length ? "" : "OK", tool_calls: toolCalls }, done: true, load_duration: 1_234_000_000, eval_count: 40, eval_duration: 1_000_000_000 });
      }
      if (req.method === "GET" && req.url === "/api/ps") return send(200, { models: lastModel ? [{ name: lastModel, model: lastModel, context_length: 32768 }] : [] });
      send(404, { error: "not found" });
    });
  });
  const host = process.env.E2E_OLLAMA_FIXTURE_HOST || "127.0.0.1";
  return new Promise((resolve) => server.listen(0, host === "127.0.0.1" ? host : "0.0.0.0", () => resolve({ server, hits, chats, url: `http://${host}:${server.address().port}` })));
}

export async function runOllamaProfileChecks({ base, token, step, check, openPage, out }) {
  const fixture = await startFixture();
  const auth = { Authorization: `Bearer ${token}` };
  try {
    for (const locale of ["en", "ko"]) {
      const t = T[locale];
      const name = `e2e ollama ${locale}`;
      const initialHits = fixture.hits.length;
      const p = await openPage(locale, token);
      const llmCalls = [];
      p.on("request", (r) => {
        const path = new URL(r.url()).pathname;
        if (/^\/api\/llm\/(test|models|profiles)/.test(path) && r.method() === "POST") llmCalls.push(path);
      });
      try {
        await step(`[${locale}] Ollama templates fill native defaults, clear the unsaved key and send nothing`, async () => {
          await p.goto(`${base}/system/llm`);
          await p.getByRole("button", { name: t.i.m0728, exact: true }).click();
          const sheet = p.getByRole("dialog");
          await sheet.getByRole("heading", { name: t.i.m1382, exact: true }).waitFor();
          await p.fill("#p-name", name);
          await p.fill("#p-proxy", "http://127.0.0.1:9");
          await p.fill("#p-session-header", "x-e2e-session");
          await p.fill("#p-api-key", "sk-cloud-unsaved");
          const routes = [
            ["ollamaHost", "http://127.0.0.1:11434", TOOL_MODEL],
            ["ollamaDockerHost", "http://host.docker.internal:11434", TOOL_MODEL],
            ["ollamaCompose", "http://ollama:11434", "qwen2.5-coder:14b"],
          ];
          for (const [key, url, model] of routes) {
            await p.locator("#p-preset").click();
            await p.getByRole("option", { name: t.p[key], exact: true }).click();
            const values = await p.evaluate(() => Object.fromEntries(["p-name", "p-proxy", "p-session-header", "p-api-key", "p-base-url", "p-model", "p-cw", "p-max-concurrent", "p-keep-alive", "p-max-tokens"].map((id) => [id, document.getElementById(id)?.value])));
            check(values["p-base-url"] === url && values["p-model"] === model, `${key} route ${values["p-base-url"]} ${values["p-model"]}`);
            check(values["p-cw"] === "32" && values["p-max-concurrent"] === "1" && values["p-keep-alive"] === "30m" && values["p-max-tokens"] === "4096", `${key} defaults ${JSON.stringify(values)}`);
            check(values["p-api-key"] === "", `${key} kept the unsaved cloud key`);
            check(values["p-name"] === name && values["p-proxy"] === "http://127.0.0.1:9" && values["p-session-header"] === "x-e2e-session", `${key} lost preferences`);
            await sheet.getByRole("combobox", { name: t.i.m1388, exact: true }).getByText(t.p.ollamaFormat, { exact: true }).waitFor();
          }
          await sheet.getByText(t.p.ollamaCloudWarning, { exact: true }).waitFor();
          await p.screenshot({ path: `${out}/ollama-template-${locale}.png` });
          check(llmCalls.length === 0, `template selection sent ${llmCalls.join(",")}`);
          check(fixture.hits.length === initialHits, `template selection contacted the endpoint: ${fixture.hits.slice(initialHits).join(",")}`);
        });

        await step(`[${locale}] cloud formats and GLM templates stay selectable and manually editable`, async () => {
          const sheet = p.getByRole("dialog");
          await p.locator("#p-preset").click();
          await p.getByRole("option", { name: t.p.general, exact: true }).click();
          check(await p.inputValue("#p-base-url") === "https://api.z.ai/api/paas/v4" && await p.inputValue("#p-model") === "glm-5.3", "GLM template changed");
          await sheet.getByRole("combobox", { name: t.i.m1388, exact: true }).click();
          for (const label of ["Anthropic", "OpenAI (Chat Completions)", "OpenAI (Responses API)", t.p.ollamaFormat]) {
            await p.getByRole("option", { name: label, exact: true }).waitFor();
          }
          await p.getByRole("option", { name: "OpenAI (Responses API)", exact: true }).click();
          await p.fill("#p-base-url", "https://gateway.example.invalid/v1");
          await p.fill("#p-model", "custom-model");
          check(await p.inputValue("#p-base-url") === "https://gateway.example.invalid/v1" && await p.inputValue("#p-model") === "custom-model", "manual fields not editable");
          check(llmCalls.length === 0 && fixture.hits.length === initialHits, "manual edits sent requests");
        });

        await step(`[${locale}] keyless Ollama profile loads metadata, diagnoses, saves and activates against the fixture`, async () => {
          const sheet = p.getByRole("dialog");
          await p.locator("#p-preset").click();
          await p.getByRole("option", { name: t.p.ollamaHost, exact: true }).click();
          await p.fill("#p-proxy", "");
          await p.fill("#p-base-url", fixture.url);
          await sheet.getByRole("button", { name: t.i.m1390, exact: true }).click();
          const list = p.locator('[data-slot="popover-content"]');
          // Assert metadata per model row so one row's badges cannot satisfy another row's checks.
          const row = (model) => list.getByRole("button").filter({ has: p.getByText(model, { exact: true }) });
          for (const text of ["32.8B", "Q4_K_M", "tools"]) await row(TOOL_MODEL).getByText(text, { exact: true }).waitFor();
          for (const text of ["8.0B", "Q4_0", t.p.noToolsBadge]) await row(NO_TOOLS_MODEL).getByText(text, { exact: true }).waitFor();
          check(await row(TOOL_MODEL).getByText(t.p.noToolsBadge, { exact: true }).count() === 0, "tool-capable row shows the no-tools badge");
          check(await row(NO_TOOLS_MODEL).getByText("tools", { exact: true }).count() === 0, "no-tools row shows the tools badge");
          await list.getByText(NO_TOOLS_MODEL, { exact: true }).click();
          await sheet.getByText(t.p.noTools, { exact: true }).waitFor();
          await p.fill("#p-model", "absent-model:1b");
          await sheet.getByRole("button", { name: t.i.m1418, exact: true }).click();
          await sheet.getByText("ollama pull absent-model:1b", { exact: true }).waitFor();
          await p.fill("#p-model", TOOL_MODEL);
          check(!(await sheet.getByText(t.p.noTools, { exact: true }).isVisible()), "tools warning shown for a tool-capable model");
          await sheet.getByRole("button", { name: t.i.m1418, exact: true }).click();
          await sheet.getByText(t.p.diagToolsOk, { exact: true }).waitFor();
          for (const text of [t.p.diagContextOk.replace("{actual}", "32768").replace("{requested}", "32768"), t.p.diagLoadValue.replace("{ms}", "1234"), t.p.diagSpeedValue.replace("{tps}", "40.0")]) {
            await sheet.getByText(text, { exact: true }).waitFor();
          }
          const chat = fixture.chats.at(-1);
          check(chat.options?.num_ctx === 32768 && chat.options?.num_predict === 4096 && chat.keep_alive === "30m", `native request ${JSON.stringify({ options: chat.options, keep_alive: chat.keep_alive })}`);
          await p.screenshot({ path: `${out}/ollama-diagnostics-${locale}.png` });
          await sheet.getByRole("button", { name: t.i.m0728, exact: true }).click();
          await sheet.waitFor({ state: "hidden" });
          const profiles = (await (await p.request.get(`${base}/api/llm/profiles`, { headers: auth })).json()).profiles;
          const saved = profiles.find((x) => x.name === name);
          check(saved && saved.format === "ollama" && saved.context_window_k === 32 && saved.llm_max_concurrent === 1 && saved.keep_alive === "30m" && saved.max_tokens === 4096 && !saved.api_key_hint && !saved.proxy && saved.session_header_key === "x-e2e-session", `saved ${JSON.stringify(saved)}`);
          const card = p.locator('[data-slot="card"]').filter({ has: p.getByText(name, { exact: true }) });
          await card.getByText(t.p.keyless, { exact: true }).waitFor();
          await card.getByText(t.i.m1339, { exact: true }).waitFor();
          await card.getByRole("button", { name: t.i.m1431, exact: true }).click();
          await p.getByText(t.i.m1419.replace("{p0}", name), { exact: true }).waitFor();
          const active = (await (await p.request.get(`${base}/api/llm/profiles`, { headers: auth })).json()).profiles.find((x) => x.name === name);
          check(active?.is_default === true, "keyless Ollama profile did not activate");
        });

        await step(`[${locale}] local-only LLM policy is visible, explained and off by default`, async () => {
          const settings = await (await p.request.get(`${base}/api/settings`, { headers: auth })).json();
          check(settings.local_llm_only === false, `local_llm_only defaulted to ${settings.local_llm_only}`);
          await p.goto(`${base}/system/settings`);
          const toggle = p.locator("#local-llm-only:not([disabled])");
          await toggle.waitFor();
          check(await toggle.getAttribute("aria-checked") === "false", "local-only switch is on by default");
          await p.getByText(t.s.localOnlyTitle, { exact: true }).waitFor();
          await p.getByText(t.s.localOnlyEnv, { exact: true }).waitFor();
        });

        await step(`[${locale}] compatible local keyless health differs from a public cloud profile missing its key`, async () => {
          const localName = `e2e local compatible ${locale}`;
          const publicName = `e2e public missing key ${locale}`;
          for (const [profileName, baseURL] of [[localName, fixture.url], [publicName, "http://203.0.113.7:11434/v1"]]) {
            const response = await p.request.post(`${base}/api/llm/profiles`, { headers: auth, data: {
              name: profileName, format: "openai", model: TOOL_MODEL, base_url: baseURL, api_key: "",
            } });
            check(response.ok(), `profile creation failed: ${response.status()}`);
          }
          const profiles = (await (await p.request.get(`${base}/api/llm/profiles`, { headers: auth })).json()).profiles;
          check(profiles.find((x) => x.name === localName)?.api_key_optional === true, "local compatible key is not optional");
          check(profiles.find((x) => x.name === publicName)?.api_key_optional === false, "public cloud key became optional");
          await p.goto(`${base}/system/llm`);
          const card = (profileName) => p.locator('[data-slot="card"]').filter({ has: p.getByText(profileName, { exact: true }) });
          await card(localName).getByText(t.p.keyless, { exact: true }).waitFor();
          check(await card(localName).getByText(t.i.m1334, { exact: true }).count() === 0, "local compatible profile is mislabeled as missing a key");
          await card(publicName).getByText(t.i.m1334, { exact: true }).waitFor();
        });
      } finally {
        await p.context().close();
      }
    }
  } finally {
    fixture.server.close();
  }
}
