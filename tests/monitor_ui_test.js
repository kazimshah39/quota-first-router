// Focused browser-controller regression using only Node's standard library.
// Real Chrome/CPA/OMC acceptance is separate; this tests abort/logout races and
// standalone observer recursion without introducing a browser framework.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
class Element {
  constructor(tag = 'div') { this.tagName = tag; this.children = []; this.handlers = {}; this.value = ''; this.hidden = false; this.dataset = {}; this.attributes = {}; this._text = ''; }
  set textContent(value) { this._text = String(value); this.children = []; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; this._text = ''; }
  get firstChild() { return this.children[0]; }
  querySelector(tag) { return this.findAll(tag)[0]; }
  findAll(tag) { return this.children.flatMap(c => [...(c.tagName === tag ? [c] : []), ...c.findAll(tag)]); }
  addEventListener(type, fn) { this.handlers[type] = fn; }
  setAttribute(k,v) { this.attributes[k] = v; }
  focus() {}
  click() { this.handlers.click?.({preventDefault(){}}); }
}
async function main() {
  const ids = new Map(); const element = id => { if (!ids.has(id)) ids.set(id,new Element()); return ids.get(id); };
  let requests = [], observations = 0, writes = 0, color = 'dark', nextTimer = 0;
  const timers = new Map();
  const document = { hidden:false,documentElement:new Element(),createElement:tag=>new Element(tag),getElementById:element,addEventListener(type,fn){this[type]=fn;} };
  const context = { document, URL, Uint8Array, TextEncoder, TextDecoder, atob, AbortController, Date, JSON, console,
    localStorage:{getItem(){return null;},setItem(){writes++;}}, location:{origin:'http://fixture',host:'fixture'},
    MutationObserver:class {observe(){observations++;}}, matchMedia:()=>({get matches(){return color==='dark';},addEventListener(type,fn){document.themeChange=fn;}}),
    setTimeout(fn,delay){timers.set(++nextTimer,{fn,delay});return nextTimer;},
    clearTimeout(id){timers.delete(id);},
    fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));},
  };
  context.window=context; context.parent=context;
  vm.runInNewContext(fs.readFileSync(require('node:path').join(__dirname,'../web/monitor.js'),'utf8'),context);
  assert.equal(observations,0,'standalone must not observe itself');
  assert.equal(document.documentElement.dataset.theme,'dark');
  color='light';document.themeChange();
  assert.equal(document.documentElement.dataset.theme,'light','follows browser preference');
  const submit=key=>{element('key').value=key;element('login').handlers.submit({preventDefault(){}});};
  const tick=()=>new Promise(resolve=>setImmediate(resolve));
  submit('secret');assert.equal(requests.length,1);assert.equal(element('key').value,'');
  element('refresh').click();assert.equal(requests.length,1,'one request in flight');
  element('signout').click();assert.equal(requests[0].options.signal.aborted,true);
  const snapshot={version:'0.2.0',server_time:new Date().toISOString(),mode:'active',state:'healthy',accounts:[],total_accounts:0,fresh_accounts:0,picks:0,routed:0,poll_interval:'1m0s',history:{epoch:'one',started_at:new Date().toISOString(),decisions:[],incidents:[],decision_evictions:0,incident_evictions:0,candidate_truncations:0}};
  requests[0].resolve({ok:true,status:200,json:async()=>snapshot});await tick();
  assert.equal(element('private').hidden,true,'late response must not restore logged-out diagnostics');
  assert.equal(element('history-note').textContent,'');
  const now=Date.parse(snapshot.server_time), after=minutes=>new Date(now+minutes*60000).toISOString();
  const quota = {plan_type:'plus',five_hour:{utilization:20,resets_at:after(19)},seven_day:{utilization:62,resets_at:after(2*1440+4*60)},model_windows:{'gpt-example':[{utilization:100,resets_at:after(-1)}]}};
  const populated = {...snapshot, total_accounts:4, fresh_accounts:3, history:{...snapshot.history, decisions:[{model:'gpt-example', at:snapshot.server_time, sequence:1, decision:{auth_id:'primary', candidates:[{auth_id:'primary', priority:20, reason:'ready'}]}}]}, accounts:[
    {id:'primary', availability:'ready', quota, warmup_status:'confirmed'},
    {id:'cached', availability:'stale', quota:{five_hour:{utilization:0,resets_at:after(131)}}, warmup_status:'<unexpected status>'},
    {id:'monthly', availability:'ready', quota:{five_hour_absent:true,seven_day:{utilization:76}}},
    {id:'unread', availability:'unknown'},
  ]};
  submit('secret');requests[1].resolve({ok:true,status:200,json:async()=>populated});await tick();
  assert.equal(element('private').hidden,false);
  const list=element('accounts');
  assert.deepEqual(list.findAll('progress').map(bar=>[bar.value,bar.dataset.tone]),[[80,'good'],[38,'low'],[0,'critical'],[100,'cached'],[24,'critical']]);
  assert.equal(list.children[3].findAll('progress').length,0,'unknown is not 0%');
  assert.match(list.children[2].textContent,/Monthly allowance/);
  assert.doesNotMatch(list.children[2].textContent,/5 hours/);
  assert.match(list.children[0].textContent,/Selected/);
  assert.doesNotMatch(list.children[0].textContent,/Quota usable|Last offered/);
  assert.match(list.textContent,/19m left/);
  assert.match(list.textContent,/2h 11m left/);
  assert.match(list.textContent,/2d 4h left/);
  assert.match(list.textContent,/Waiting for reset check/);
  assert.equal(element('model-filter').hidden,true);
  assert.equal(element('signout').hidden,false);
  assert.equal(element('activity').children.length,1,'latest selection is not rendered twice');
  assert.equal(element('history-note').hidden,true);
  const details=list.findAll('details');
  assert.equal(details.every(detail=>detail.open===false),true);
  assert.match(details[0].textContent,/gpt-example/);
  assert.match(details[0].textContent,/Warmup: Window confirmed/);
  assert.doesNotMatch(list.children[0].children[0].textContent,/Warmup/);
  assert.doesNotMatch(list.textContent,/unexpected status/);
  details[0].open=true;
  element('refresh').click();requests[2].resolve({ok:true,status:200,json:async()=>populated});await tick();
  assert.equal(list.findAll('details')[0].open,true,'auto-refresh must not close expanded account');
  const named = {...populated, accounts:[
    {id:'codex-9router-527e014a-alex+work@example.test.json', availability:'exhausted', quota, priority:0},
    {id:'codex-abcd1234-different@example.test-plus.json', email:'alex+work@example.test', availability:'ready', quota, priority:10},
    {id:'codex-12345678-non-email.json', availability:'unknown'},
    {id:'codex-aabb1234-other@example.test-free.json', email:'user-name@googlemail.com', availability:'ready', priority:20, quota:{plan_type:'free'}},
  ]};
  element('refresh').click();requests[3].resolve({ok:true,status:200,json:async()=>named});await tick();
  assert.deepEqual(list.findAll('h3').map(title=>title.textContent), ['codex-12345678-non-email.json','alex+work@example.test','user-name@gmail.com','alex+work@example.test']);
  assert.doesNotMatch(list.textContent,/Tier (unknown|\d)/);
  assert.match(list.children[1].children[0].textContent,/Plus/);
  assert.match(list.children[2].children[0].textContent,/Free/);
  for (const [i, priority] of [[1,0],[2,20],[3,10]]) {
    assert.doesNotMatch(list.children[i].children[0].textContent,/priority/);
    assert.match(list.children[i].querySelector('details').textContent,new RegExp(`Routing priority: ${priority}`));
  }
  assert.equal(list.children[1].dataset.exhausted,'true');
  assert.equal(list.children[1].findAll('span').find(el=>el.className==='quota-alert').attributes['aria-label'],'Quota exhausted');
  assert.doesNotMatch(list.children[1].textContent,/Quota exhausted/);
  list.findAll('details')[1].open=true;
  const history={...named.history,decisions:[...named.history.decisions,...Array.from({length:7},(_,i)=>({model:'another-model',sequence:i+2,at:after(i),decision:{reason:`fixture_reason_${i===6?5:i}`}}))],incidents:[{kind:'quota',code:'quota_http_503',at:after(-2),last_at:after(8),sequence:1,account:named.accounts[0].id}]};
  element('refresh').click();requests[4].resolve({ok:true,status:200,json:async()=>({...named,history})});await tick();
  assert.deepEqual(list.findAll('details').map(detail=>detail.open), [false,true,false,false], 'same email must not merge disclosure state');
  const activity=element('activity');
  assert.equal(activity.children.length,6);
  assert.equal(activity.children[0].dataset.kind,'issue','sort coalesced incidents by latest occurrence');
  assert.equal(element('model-filter').hidden,false);
  assert.match(activity.children[1].textContent,/×2/,'consecutive identical routing decisions are grouped');
  element('activity-more').click();assert.equal(activity.children.length,8);
  element('activity-more').click();assert.equal(activity.children.length,6);
  element('refresh').click();requests[5].resolve({ok:false,status:403});await tick();
  assert.equal(list.findAll('progress').length,0,'forbidden removes previous quota indicators');
  assert.equal(element('private').hidden,true);assert.equal(element('history-note').textContent,'');assert.equal(element('model').textContent,'Latest model');assert.equal(element('activity').children.length,0);
  assert.equal(writes,0,'must not persist entered key');
  submit('secret');document.hidden=true;document.visibilitychange();assert.equal(requests[6].options.signal.aborted,true,'hidden page aborts');
  document.hidden=false;document.visibilitychange();
  assert.equal(requests.length,7,'wait for the previous request to settle before resuming');
  requests[6].resolve({ok:true,status:200,json:async()=>snapshot});await tick();assert.equal(element('private').hidden,true);
  assert.equal(requests.length,8,'quickly returning visible starts a fresh read');
  requests[7].resolve({ok:true,status:200,json:async()=>snapshot});await tick();
  assert.equal(element('private').hidden,false);
  assert.deepEqual([...timers.values()].map(timer=>timer.delay),[5000],'a visible tab keeps refreshing');
  document.hidden=true;document.visibilitychange();
  assert.equal(timers.size,0,'hidden tab cancels its refresh timer');
  assert.equal(requests.length,8);
  document.hidden=false;document.visibilitychange();
  assert.equal(requests.length,9,'returning visible reads immediately');
  console.log('PASS monitor: remaining-quota bars, thresholds, monthly/unknown/stale states, persistent disclosure, theme, logout race, auth clearing and single-flight refresh');
}
main().catch(error=>{console.error(error);process.exitCode=1;});
