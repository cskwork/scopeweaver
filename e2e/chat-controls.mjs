import assert from "node:assert/strict";
import {execFileSync} from "node:child_process";
import {createServer} from "node:http";
import {chromium,request} from "playwright";
import {en,ko} from "../web/src/i18n/messages/interface.ts";
import {interface as zh} from "../web/src/i18n/messages/zh-Hans.ts";
const labels={en,ko,"zh-Hans":zh};

export async function runChatQueueChecks({base,token,step,check,openPage,recoverInput}){
 const api=await request.newContext({baseURL:base,extraHTTPHeaders:{Authorization:`Bearer ${token}`}});
 for(const locale of ["en","ko","zh-Hans"])for(const width of [1440,390])for(const surface of ["standalone","task"]){
  if(process.env.CHAT_QUEUE_CASE && `${locale}/${width}/${surface}`!==process.env.CHAT_QUEUE_CASE)continue;
  const t=labels[locale];let path,url,task;
  if(surface==="standalone"){const r=await api.post("/api/conversations",{data:{agent_key:"auto",title:"Queue browser fixture"}});check(r.ok(),"conversation creation failed");const conv=await r.json();path=`/api/conversations/${conv.id}/controls`;url=`${base}/chat?c=${conv.id}`}
  else{const r=await api.post("/api/tasks",{data:{description:"Queue task fixture",goal:"Keep idle",seed_first_intent:false}});check(r.ok(),"queue task failed");task=await r.json();check((await api.post(`/api/tasks/${task.id}/control`,{data:{action:"pause"}})).ok(),"fixture pause failed");path=`/api/tasks/${task.id}/chat/controls?seg=0`;url=`${base}/function/tasks/detail?id=${task.id}`}
  const actionPath=(action)=>{const [p,q]=path.split("?");return `${p}/${action}${q?`?${q}`:""}`};
  const state=await (await api.get(path)).json();check((await api.post(actionPath("pause"),{data:{expected_revision:state.revision}})).ok(),"queue pause failed");
  const p=await openPage(locale,token);await p.setViewportSize({width,height:900});
  await step(`[${locale}/${width}/${surface}] queue edits, order, pause and request identity`,async()=>{
   await p.goto(url);const input=p.locator("textarea[data-chat-draft]").first();await input.waitFor();
   await p.getByRole("button",{name:t.chatResumeQueue,exact:true}).waitFor();
   const submit=async(text)=>{await input.fill(text);await p.getByRole("button",{name:t.chatQueue,exact:true}).click();await p.waitForFunction(()=>document.querySelector("textarea[data-chat-draft]")?.value==="")};
   await submit("first");await submit("second");await submit("third");
   const rows=p.locator("[data-queue-input]");await rows.nth(2).waitFor();
   await rows.nth(2).getByRole("button",{name:t.chatMoveUp,exact:true}).click();
   await p.waitForFunction(()=>document.querySelectorAll("[data-queue-input] summary")[1]?.textContent==="third");
   await rows.nth(0).getByRole("button",{name:t.chatQueueRemove,exact:true}).click();await p.waitForFunction(()=>document.querySelectorAll("[data-queue-input]").length===2);
   await input.fill("pre-edit draft");await rows.nth(0).getByRole("button",{name:t.chatQueueEdit,exact:true}).click();await p.getByRole("dialog").locator("textarea").fill("cancelled edit");await p.getByRole("button",{name:t.chatCancelEdit,exact:true}).click();check(await input.inputValue()==="pre-edit draft","cancel overwrote composer draft");
   await rows.nth(0).getByRole("button",{name:t.chatQueueEdit,exact:true}).click();await p.getByRole("dialog").locator("textarea").fill("third revised");await p.getByRole("button",{name:t.chatQueueSave,exact:true}).click();await p.waitForFunction(()=>document.querySelector("[data-queue-input] summary")?.textContent==="third revised");
   const clientIDs=[];let lose=true;
   await p.route("**/controls/inputs**",async route=>{if(route.request().method()!=="POST")return route.continue();clientIDs.push(route.request().postDataJSON().client_request_id);if(lose){lose=false;await route.fetch();await route.abort("failed")}else await route.continue()});
   await input.fill("lost response");await p.getByRole("button",{name:t.chatQueue,exact:true}).click();await rows.nth(2).waitFor();check(await input.inputValue()==="lost response","uncertain submission lost draft");await p.getByRole("button",{name:t.chatQueue,exact:true}).click();await p.waitForFunction(()=>document.querySelector("textarea[data-chat-draft]")?.value==="");check(clientIDs.length===2&&clientIDs[0]===clientIDs[1],"lost response changed client identity");check(await rows.count()===3,"duplicate queued input");
   const other=await openPage(locale,token);await other.goto(url);await other.locator("[data-queue-input]").nth(2).waitFor();check(await other.locator("[data-queue-input] summary").first().textContent()==="third revised","second window disagrees");await other.context().close();
   if(recoverInput){const inputID=await rows.last().getAttribute("data-queue-input");await recoverInput(inputID);await p.getByRole("button",{name:t.chatDiscardLost,exact:true}).waitFor();check(await p.getByRole("button",{name:t.chatResumeQueue,exact:true}).isDisabled(),"unknown run did not block Resume");await p.getByRole("button",{name:t.chatDiscardLost,exact:true}).click();await p.getByRole("button",{name:t.chatDiscardLost,exact:true}).waitFor({state:"hidden"});check(await p.getByRole("button",{name:t.chatResumeQueue,exact:true}).isEnabled(),"discard must leave Resume available");check(await p.getByRole("button",{name:t.chatDiscardLost,exact:true}).count()===0,"discard did not acknowledge unknown input")}

  });await p.context().close();
 }
 await api.dispose();
}

if(process.argv[1]?.endsWith("chat-controls.mjs")){
 const dsn=new URL((process.env.ARTEX_PG_DSN??"").replace("@/","@localhost/"));assert.ok(dsn.pathname.slice(1).startsWith("scopeweaver_test_"));
 const base=process.env.SCOPEWEAVER_URL??"http://127.0.0.1:18790";const api=await request.newContext({baseURL:base});
 await api.post("/api/auth/init",{data:{password:"chat-controls-pass-1234"}});const login=await api.post("/api/auth/login",{data:{username:"ARTEX",password:"chat-controls-pass-1234"}});assert.ok(login.ok());const {token}=await login.json();
 const fixture=createServer(async(req,res)=>{for await(const chunk of req){};res.writeHead(200,{"Content-Type":"application/json"});res.end(JSON.stringify({id:"fixture",choices:[{message:{role:"assistant",content:"Fixture response"},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:4,total_tokens:14}}))});await new Promise(resolve=>fixture.listen(0,"127.0.0.1",resolve));
 const prior=await (await api.get("/api/tasks",{headers:{Authorization:`Bearer ${token}`}})).json();for(const task of prior.tasks??[])await api.post(`/api/tasks/${task.id}/control`,{headers:{Authorization:`Bearer ${token}`},data:{action:"pause"}});
 const configured=await api.post("/api/llm",{headers:{Authorization:`Bearer ${token}`},data:{provider:"openai",base_url:`http://127.0.0.1:${fixture.address().port}/v1`,api_key:"fixture",model:"fixture",context_window_k:32}});assert.ok(configured.ok(),await configured.text());
 const browser=await chromium.launch();let failures=0;try{await runChatQueueChecks({base,token,recoverInput:async id=>{assert.match(id,/^[a-f0-9]+$/);execFileSync("psql",[dsn.href,"-c",`BEGIN; UPDATE chat_control_inputs SET status='unknown',revision=revision+1 WHERE id='${id}'; UPDATE chat_control_gates SET revision=revision+1 WHERE gate_key=(SELECT gate_key FROM chat_control_inputs WHERE id='${id}'); COMMIT`],{stdio:"ignore"})},check:assert.ok,step:async(name,fn)=>{try{await fn();console.log(`PASS ${name}`)}catch(error){failures++;console.error(`FAIL ${name}: ${error.stack}`);if(process.env.CHAT_QUEUE_RED)throw error}},openPage:async(locale,jwt)=>{const context=await browser.newContext();context.setDefaultTimeout(10000);await context.addInitScript(([l,t])=>{localStorage.setItem("scopeweaver.locale",l);localStorage.setItem("artex_token",t)},[locale,jwt]);return context.newPage()}})}finally{await browser.close();await api.dispose();await new Promise(resolve=>fixture.close(resolve))}if(failures)process.exitCode=1;
}
