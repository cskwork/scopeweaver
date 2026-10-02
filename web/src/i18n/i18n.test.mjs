import assert from "node:assert/strict";
import test from "node:test";
import fs from "node:fs";
import path from "node:path";
import { catalogs } from "./catalog.ts";
import { normalizeLocale, readCookieLocale, withLocaleQuery, withLocaleHeaders } from "./config.ts";
import { placeholders, tags, interpolate, createFormatters } from "./format.ts";
import { __resetLocaleForTests, getLocale, setLocale, initializeLocale, translate, translateIn, subscribeLocale, localeHeaders } from "./runtime.ts";
import { cloneDemoFixture } from "./demo-fixture.ts";
import { localizedFetch } from "./request.ts";
import { mentionKinds, mentionToken, selectedMentions, mentionSearch } from "../lib/chat-mentions.ts";
import { sourceFiles, scanFile, SRC_ROOT } from "./han-scan.mjs";

function browser(stored = null, cookie = "") {
  const values = new Map(stored ? [["scopeweaver.locale", stored]] : []);
  globalThis.window = { localStorage: { getItem: (key) => values.get(key) ?? null, setItem: (key,value) => values.set(key,value) } };
  globalThis.document = { cookie, documentElement: { lang: "en" } };
  return values;
}
function reset() { delete globalThis.window; delete globalThis.document; __resetLocaleForTests(); }

test("English is the default; unsupported and malformed preferences safely fall back", () => {
  reset(); assert.equal(getLocale(), "en");
  assert.equal(normalizeLocale("ko-KR"), "ko");
  assert.equal(normalizeLocale("zh-CN"), null);
  assert.equal(readCookieLocale("scopeweaver_locale=%E0%A4%A"), null);
  browser("unsupported", "scopeweaver_locale=ko"); initializeLocale(); assert.equal(getLocale(), "ko");
  reset(); browser("unsupported", "scopeweaver_locale=%bad"); initializeLocale(); assert.equal(getLocale(), "en"); reset();
});

test("persisted Korean starts with the English hydration snapshot and initializes afterward", () => {
  reset(); browser("ko");
  assert.equal(getLocale(), "en"); assert.equal(translate("common.save"), "Save");
  assert.equal(localeHeaders()["Accept-Language"], "ko", "first API call honors persisted preference before effects");
  let calls=0; const unsubscribe=subscribeLocale(()=>calls++); initializeLocale();
  assert.equal(getLocale(), "ko"); assert.equal(translate("common.save"), "저장"); assert.equal(calls,1);
  assert.match(document.cookie, /scopeweaver_locale=ko; Path=\//); assert.equal(document.documentElement.lang,"ko");
  setLocale("en"); assert.equal(window.localStorage.getItem("scopeweaver.locale"),"en"); assert.equal(calls,2);
  unsubscribe(); reset();
});

test("both catalogs are complete and preserve interpolation fields", () => {
  assert.deepEqual(Object.keys(catalogs.en).sort(), Object.keys(catalogs.ko).sort());
  assert.ok(Object.keys(catalogs.en).length >= 3000);
  for(const key of Object.keys(catalogs.en)) {
    assert.ok(catalogs.en[key].length, key); assert.ok(catalogs.ko[key].length,key);
    assert.deepEqual(placeholders(catalogs.en[key]), placeholders(catalogs.ko[key]),key);
    assert.doesNotMatch(catalogs.en[key], /Untranslated|미번역|\p{Script=Han}/u,key);
    assert.doesNotMatch(catalogs.ko[key], /Untranslated|미번역|\p{Script=Han}/u,key);
  }
  assert.equal(interpolate("Task {id}: {name}",{id:7,name:"사용자 原文 {unchanged}"}),"Task 7: 사용자 原文 {unchanged}");
  assert.equal(interpolate("{missing}",{}),"{missing}");
});

test("locale transport preserves caller headers, queries, and fragments", async () => {
  assert.equal(withLocaleQuery("/api/report?task=3#section","ko"),"/api/report?task=3&lang=ko#section");
  assert.equal(withLocaleQuery("/api/report?lang=en","ko"),"/api/report?lang=en");
  assert.deepEqual(withLocaleHeaders("ko", {Authorization:"Bearer demo", "Content-Type":"application/json"}),{"Accept-Language":"ko",Authorization:"Bearer demo","Content-Type":"application/json"});
  reset(); browser(); setLocale("ko"); const original=globalThis.fetch; let observed;
  globalThis.fetch=async(input,init)=>{observed={input,init};return new Response("ok")};
  try {
    await localizedFetch("/api/report?task=1",{headers:{Authorization:"Bearer example"}});
    assert.equal(observed.input,"/api/report?task=1&lang=ko");
    assert.equal(new Headers(observed.init.headers).get("accept-language"),"ko");
    assert.equal(new Headers(observed.init.headers).get("authorization"),"Bearer example");
  } finally {globalThis.fetch=original;reset();}
});

test("authored demo data changes language; edited and cloned user values do not", () => {
  reset(); browser();
  const fixture=cloneDemoFixture({title:"Dashboard",nested:[{name:"Save"}]},true);
  assert.equal(fixture.title,"Dashboard"); setLocale("ko"); assert.equal(fixture.title,"대시보드"); assert.equal(fixture.nested[0].name,"저장");
  Object.assign(fixture,{title:"My dashboard 原文"}); const runtimeCopy=cloneDemoFixture(fixture);
  setLocale("en"); assert.equal(fixture.title,"My dashboard 原文");assert.equal(runtimeCopy.title,"My dashboard 原文");
  fixture.title="Save"; const editedCopy=cloneDemoFixture(fixture); setLocale("ko");assert.equal(editedCopy.title,"Save","user text matching a catalog entry is still user text");reset();
});

test("localized mention labels preserve legacy wire tokens and user content", () => {
  reset();browser();setLocale("ko");assert.equal(mentionKinds[0].label,"취약점");
  assert.equal(mentionSearch("취약점 로그인").kind,"finding");assert.equal(mentionSearch("漏洞test").kind,"finding");
  const token=mentionToken({kind:"finding",id:12,label:"사용자 原文",description:""});assert.equal(token,"@[漏洞#12 사용자 原文]");
  assert.equal(selectedMentions(token)[0].label,"취약점 #12 · 사용자 原文");reset();
});

test("date and number formatters follow the selected locale", () => {
  const date=new Date("2026-10-02T00:00:00Z");
  const en=createFormatters("en"), ko=createFormatters("ko");
  assert.notEqual(en.date(date,{timeZone:"UTC"}),ko.date(date,{timeZone:"UTC"}));
  assert.equal(ko.number(12345),new Intl.NumberFormat("ko-KR").format(12345));
});

test("authored UI contains no untranslated Han literals; legacy parsers are explicit exceptions", () => {
  const legacyWire=new Set(["漏洞","资产","企业","接口","应用","域名","子域名","服务"]);
  const failures=[];
  for(const file of sourceFiles()) {
    const rel=path.relative(SRC_ROOT,file);
    if(rel.endsWith(".test.mjs"))continue; // Tests intentionally preserve multilingual user input.
    for(const hit of scanFile(file)) {
      if(rel==="lib/chat-mentions.ts" && legacyWire.has(hit.text))continue;
      if(rel==="components/approval-records.tsx" && hit.text==="/^\\[(?:模型|Model|모델)\\]\\s*/")continue;
      if(rel==="components/transcript.tsx" && hit.text==="/(?:工具|Tool|도구)\\s+(\\S+)\\s+(?:请求|requests?|요청)/i")continue;
      if(rel==="lib/company-scope.ts" && hit.text==="/icp|备案/i")continue;
      failures.push(`${rel}:${hit.line} ${hit.text}`);
    }
  }
  assert.deepEqual(failures,[]);
});

test("authored source and build configuration comments are English", async () => {
  const { hasHanComment } = await import("./han-scan.mjs");
  const files=[...sourceFiles(),path.resolve("next.config.mjs")];
  assert.deepEqual(files.filter(file=>hasHanComment(file)).map(file=>path.relative(SRC_ROOT,file)),[]);
});


test("dashboard count and token phrases preserve natural English and Korean spacing", () => {
  assert.equal(translateIn("en", "app.metricValue", { label: "Critical", value: 2 }), "Critical 2");
  assert.equal(translateIn("ko", "app.metricValue", { label: "심각", value: 2 }), "심각 2");
  const tokens = { input: "2.7M", cache: "1.7M", output: "211.7k" };
  assert.equal(translateIn("en", "app.tokenSummary", tokens), "In 2.7M (including cache 1.7M) · Out 211.7k");
  assert.equal(translateIn("ko", "app.tokenSummary", tokens), "입력 2.7M(캐시 1.7M 포함) · 출력 211.7k");
});


test("rich settings paragraphs preserve emphasis and translated theme announcements", () => {
  for (const key of Object.keys(catalogs.en).filter(key => key.startsWith("settings."))) {
    assert.deepEqual(tags(catalogs.en[key]), tags(catalogs.ko[key]), key);
  }
  assert.equal(translateIn("ko", "app.cycleTheme", { theme: "밝게" }), "현재 테마: 밝게. 클릭하여 테마 변경");
  assert.equal(translateIn("en", "settings.pagination", { from: 1, to: 4, total: 4 }), "1–4 / 4 records");
  assert.equal(translateIn("ko", "settings.pagination", { from: 1, to: 4, total: 4 }), "1–4 / 총 4건");
});
