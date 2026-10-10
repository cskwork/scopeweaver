// Essential browser smoke test against a running ScopeWeaver binary and its PostgreSQL database.
// It initializes the password, so the database must be fresh and disposable (scopeweaver_test_*).
//
// Usage: SCOPEWEAVER_URL=http://127.0.0.1:8787 ARTEX_PG_DSN=postgres://.../scopeweaver_test_e2e node smoke.mjs
// Requires the psql client; it toggles connections to the test database to simulate an outage.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import { chromium, request } from "playwright";

const base = process.env.SCOPEWEAVER_URL || "http://127.0.0.1:8787";
const dsn = new URL(process.env.ARTEX_PG_DSN || "");
const dbName = dsn.pathname.slice(1);
if (!dbName.startsWith("scopeweaver_test_")) throw new Error(`refusing to use non-test database "${dbName}"`);
const out = process.env.E2E_OUT || "e2e-results";
fs.mkdirSync(out, { recursive: true });

const PASSWORD = "e2e-pass-1234";
const TEXT = {
  en: { unavailable: "Cannot confirm setup status", dataError: "Data source is temporarily unavailable; please try again later", usage: "Approval token usage" },
  ko: { unavailable: "초기 설정 상태를 확인할 수 없습니다", dataError: "데이터 소스를 일시적으로 사용할 수 없습니다. 잠시 후 다시 시도하세요", usage: "승인 토큰 사용량" },
};

function sql(query, database = "postgres") {
  const admin = new URL(dsn);
  admin.pathname = `/${database}`;
  return execFileSync("psql", [admin.href, "-tAc", query], { encoding: "utf8" }).trim();
}
function setDatabaseAvailable(on) {
  sql(`ALTER DATABASE ${dbName} WITH ALLOW_CONNECTIONS ${on}`);
  if (!on) sql(`SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE datname = '${dbName}'`);
}
function check(ok, message) {
  if (!ok) throw new Error(message);
}

const failures = [];
const pageErrors = [];
async function step(name, fn) {
  try {
    await fn();
    console.log(`PASS ${name}`);
  } catch (err) {
    failures.push(name);
    console.log(`FAIL ${name}: ${err.message.split("\n")[0]}`);
  }
}

const browser = await chromium.launch();
async function openPage(locale, token) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript(([l, t]) => {
    localStorage.setItem("scopeweaver.locale", l);
    if (t) localStorage.setItem("artex_token", t);
  }, [locale, token]);
  const page = await context.newPage();
  page.on("pageerror", (err) => pageErrors.push(`${locale}: ${err.message}`));
  return page;
}

// 1. A database outage must not expose the first-time setup form.
for (const locale of ["en", "ko"]) {
  await step(`[${locale}] setup fails closed while the database is unavailable`, async () => {
    const page = await openPage(locale);
    setDatabaseAvailable(false);
    try {
      await page.goto(`${base}/setup`);
      await page.getByText(TEXT[locale].unavailable).waitFor();
      check(await page.getByText(TEXT[locale].dataError).isVisible(), "localized outage message missing");
      check((await page.locator('input[type="password"]').count()) === 0, "password form shown during outage");
      await page.screenshot({ path: `${out}/setup-unavailable-${locale}.png` });
    } finally {
      setDatabaseAvailable(true);
      await page.context().close();
    }
  });
}

// 2. First-time setup, then login with the same password.
const page = await openPage("en");
let token = "";
await step("[en] setup creates the password and login accepts it", async () => {
  await page.goto(`${base}/setup`);
  await page.fill("#password", PASSWORD);
  await page.fill("#confirm", PASSWORD);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/\/function\/tasks/);
  const second = await page.request.post(`${base}/api/auth/init`, { data: { password: "another-pass-99" } });
  check(second.status() === 403, `second setup returned ${second.status()}`);

  await page.evaluate(() => localStorage.removeItem("artex_token"));
  await page.goto(`${base}/login`);
  await page.fill("#password", PASSWORD);
  await page.locator("#agree-terms").click();
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/\/function\/tasks/);
  token = await page.evaluate(() => localStorage.getItem("artex_token"));
  check(token, "login stored no token");
});

// 3. Live events use the UI origin, so a reverse proxy only needs one public port.
await step("[en] log stream SSE stays on the UI origin", async () => {
  const stream = page.waitForResponse((r) => r.url().includes("/api/logs/stream"));
  await page.goto(`${base}/system/logs`);
  const response = await stream;
  check(new URL(response.url()).origin === new URL(base).origin, `SSE went to ${new URL(response.url()).origin}`);
  check((response.headers()["content-type"] || "").includes("text/event-stream"), "response is not an event stream");
});

// 4. Approval-model usage is counted separately and stays visible after the judge is disabled.
sql(`INSERT INTO llm_usage (ts, worker, profile_name, input_tokens, output_tokens, status) VALUES
  (now(), 'judge', 'e2e-judge', 1200, 300, 'ok'),
  (now() - interval '3 days', 'judge', 'e2e-judge', 800, 200, 'ok'),
  (now(), 'planner', 'e2e-other', 99999, 99999, 'ok')`, dbName);

await step("[api] approval usage excludes other agents and fills 30 days", async () => {
  const response = await page.request.get(`${base}/api/intercept/judge/usage?days=30`, { headers: { Authorization: `Bearer ${token}` } });
  const usage = await response.json();
  check(usage.calls === 2 && usage.input_tokens === 2000 && usage.output_tokens === 500, `totals ${JSON.stringify(usage).slice(0, 120)}`);
  check(usage.daily.length === 30 && usage.daily.filter((d) => d.calls > 0).length === 2, "daily series is not 30 zero-filled days");
  // A separate context carries no session cookie.
  const anonymous = await request.newContext();
  const denied = await anonymous.get(`${base}/api/intercept/judge/usage`);
  check(denied.status() === 401, `unauthenticated request returned ${denied.status()}`);
  await anonymous.dispose();
});

for (const locale of ["en", "ko"]) {
  await step(`[${locale}] approval usage card shows past spend while the judge is disabled`, async () => {
    const p = await openPage(locale, token);
    await p.goto(`${base}/system/intercept`);
    await p.getByRole("tab").nth(1).click();
    await p.getByText(TEXT[locale].usage).waitFor();
    const bars = p.locator("div.flex.h-16 > div");
    check((await bars.count()) === 30, `${await bars.count()} daily bars`);
    check((await p.locator("div.flex.h-16 > div.bg-violet-500\\/60").count()) === 2, "expected 2 days with usage");
    await p.screenshot({ path: `${out}/approval-usage-${locale}.png`, fullPage: true });
    await p.context().close();
  });
}

await step("no uncaught page errors", async () => check(pageErrors.length === 0, pageErrors.join(" | ")));

await browser.close();
console.log(failures.length ? `E2E failed: ${failures.length} step(s)` : "E2E passed");
process.exit(failures.length ? 1 : 0);
