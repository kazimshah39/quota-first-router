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
  querySelectorAll(selector) { return this.findAll(selector); }
  findAll(selector) { return this.children.flatMap(c => [...((selector.startsWith('.') ? c.className === selector.slice(1) : c.tagName === selector) ? [c] : []), ...c.findAll(selector)]); }
  addEventListener(type, fn) { this.handlers[type] = fn; }
  setAttribute(k,v) { this.attributes[k] = v; }
  getAttribute(k) { return this.attributes[k] ?? null; }
  focus() { this.focused = true; }
  showModal() { this.open = true; }
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
  const help = element('monitor-help'), helpButton = element('open-help');
  element('help-content').scrollTop = 80;
  helpButton.click();
  assert.equal(help.open, true);
  assert.equal(element('help-content').scrollTop, 0);
  assert.equal(requests.length, 0, 'help opens without credentials and does not fetch account data');
  help.open = false;
  help.handlers.close();
  assert.equal(helpButton.focused, true, 'closing help restores focus');
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
    {id:'primary', availability:'ready', next_refresh:after(2), quota:{...quota,read_at:after(-56/60),five_hour:{utilization:0,resets_at:after(300)}}, five_hour_idle:true},
    {id:'cached', five_hour_idle:true, availability:'stale', next_refresh:after(1), last_error:'quota_http_503', quota:{read_at:after(-6),five_hour:{utilization:0,resets_at:after(131)}}, warmup_status:'<unexpected status>'},
    {id:'monthly', availability:'ready', next_refresh:'0001-01-01T00:00:00Z', quota:{five_hour_absent:true,seven_day:{utilization:76}}},
    {id:'unread', availability:'unknown'},
  ]};
  submit('secret');requests[1].resolve({ok:true,status:200,json:async()=>populated});await tick();
  assert.equal(element('private').hidden,false);
  const list=element('accounts'), cards=()=>list.findAll('article');
  const cardIds=()=>cards().map(c=>c.dataset.accountId);
  assert.deepEqual(list.findAll('progress').map(bar=>[bar.value,bar.dataset.tone]),[[100,'cached'],[24,'critical'],[100,'good'],[38,'low']]);
  assert.equal(cards()[3].findAll('progress').length,0,'unknown is not 0%');
  assert.match(cards()[1].textContent,/Monthly allowance/);
  assert.doesNotMatch(cards()[1].textContent,/Weekly/);
  assert.match(cards()[2].textContent,/Weekly/);
  assert.doesNotMatch(list.textContent,/Long window/);
  assert.doesNotMatch(cards()[1].textContent,/5 hours/);
  assert.match(cards()[2].textContent,/Selected/);
  assert.doesNotMatch(cards()[2].textContent,/Quota usable|Last offered/);
  assert.match(list.textContent,/Timer not started/);
  assert.doesNotMatch(cards()[0].textContent,/Timer not started/,'stale evidence never gets the idle label');
  assert.match(list.textContent,/2h 11m left/);
  assert.match(list.textContent,/2d 4h left/);
  assert.match(cards()[2].textContent,/Reset passed; waiting for fresh quota/);
  assert.equal(element('filter-available').getAttribute('aria-label'),'Available (1)','only the monthly account is available; the elapsed model limit awaits evidence');
  assert.equal(element('model-filter').hidden,true);
  assert.equal(element('signout').hidden,false);
  assert.equal(element('activity').children.length,1,'latest selection is not rendered twice');
  assert.equal(element('history-note').hidden,true);
  const timing = `Last read: 56s ago · Next attempt: ${new Date(after(1)).toLocaleString()}`;
  assert.equal(element('connection').textContent,timing);
  assert.equal(list.findAll('details').length,0,'account cards do not have a Details toggle');
  assert.match(cards()[0].textContent,/Quota check failed: quota_http_503/);
  assert.doesNotMatch(list.textContent,/unexpected status|gpt-example quota limit reached/);
  element('filter-available').click();
  assert.equal(element('connection').textContent,timing,'filters must not change the shared timing');
  element('filter-all').click();
  element('refresh').click();requests[2].resolve({ok:true,status:200,json:async()=>populated});await tick();
  const named = {...populated, accounts:[
    {id:'codex-9router-527e014a-alex+work@example.test.json', availability:'exhausted', quota:{...quota, other_windows:[{utilization:100,resets_at:after(10)}], model_windows:{'gpt-example':[{utilization:100,resets_at:after(3)}]}}, priority:0},
    {id:'codex-abcd1234-different@example.test-plus.json', email:'alex+work@example.test', availability:'ready', quota:{...quota, model_windows:{}}, priority:10, warmup_status:'refused'},
    {id:'codex-12345678-non-email.json', availability:'unknown'},
    {id:'codex-aabb1234-other@example.test-free.json', email:'user-name@googlemail.com', availability:'exhausted', free_plan_excluded:true, priority:20, quota:{plan_type:'free'}},
  ]};
  element('refresh').click();requests[3].resolve({ok:true,status:200,json:async()=>named});await tick();
  assert.deepEqual(list.findAll('h3').map(title=>title.textContent), ['user-name@gmail.com','alex+work@example.test','alex+work@example.test','codex-12345678-non-email.json']);
  assert.deepEqual(list.findAll('h2').map(title=>title.textContent), ['Tier 1 (Priority 20)','Tier 2 (Priority 10)','Tier 3 (Priority 0)','Tier unknown']);
  assert.match(cards()[2].children[0].textContent,/Plus/);
  assert.match(cards()[0].children[0].textContent,/Free/);
  assert.ok(cards().every(card=>!card.textContent.includes('Priority')));
  assert.match(list.textContent,/19m left/);
  assert.match(cards()[1].textContent,/Warmup request refused/);
  assert.doesNotMatch(cards()[2].textContent,/Other quota limit reached/);
  assert.match(cards()[2].textContent,/gpt-example quota limit reached/);
  assert.equal(cards()[2].dataset.exhausted,'true');
  assert.equal(cards()[2].findAll('span').find(el=>el.className==='quota-alert').attributes['aria-label'],'Quota exhausted');
  assert.doesNotMatch(cards()[2].textContent,/Quota exhausted/);
  const filters = ['all','available','exhausted','free'].map(name=>element('filter-'+name));
  assert.deepEqual(filters.map(el=>el.getAttribute('aria-label')), ['All (4)','Available (1)','Exhausted (1)','Free (1)']);
  const savedRequests = requests.length;
  filters[3].click();
  assert.equal(filters[3].getAttribute('aria-pressed'),'true');
  assert.equal(cards().length,1);
  assert.match(list.textContent,/Excluded from automatic routing and warmup/);
  filters[2].click();
  assert.deepEqual(cardIds(),[named.accounts[0].id]);
  assert.equal(list.findAll('h2')[0].textContent,'Tier 3 (Priority 0)','filters do not renumber tiers');
  filters[1].click();
  assert.deepEqual(cardIds(),[named.accounts[1].id]);
  element('search').value='gMaIL'; element('search').handlers.input();
  assert.deepEqual(filters.map(el=>el.getAttribute('aria-label')), ['All (1)','Available (0)','Exhausted (0)','Free (1)']);
  assert.match(list.textContent,/No accounts match this search and filter/);
  filters[3].click(); assert.match(list.textContent,/user-name@gmail.com/);
  element('search').value=''; element('search').handlers.input();
  filters[0].click(); assert.equal(cards().length,4);
  assert.equal(requests.length,savedRequests,'account filters must only read cached data');
  const history={...named.history,decisions:[...named.history.decisions,...Array.from({length:7},(_,i)=>({model:'another-model',sequence:i+2,at:after(i),decision:{reason:`fixture_reason_${i===6?5:i}`}}))],incidents:[{kind:'quota',code:'quota_http_503',at:after(-2),last_at:after(8),sequence:1,account:named.accounts[0].id}]};
  element('refresh').click();requests[4].resolve({ok:true,status:200,json:async()=>({...named,history})});await tick();
  assert.equal(list.findAll('details').length,0,'refresh does not reintroduce Details');
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
  assert.equal(element('connection').textContent,'Last read: Not yet · Next attempt: Not scheduled');
  console.log('PASS monitor: quota bars, shared read timing, inline account warnings, theme, logout race and single-flight refresh');
}

async function routingOrder() {
  const origin = 'https://fixture.test', consolePath = '/console/api/v1';
  const ids = new Map(), element = id => { if (!ids.has(id)) ids.set(id,new Element()); return ids.get(id); };
  const document = {hidden:false, documentElement:new Element(), createElement:tag=>new Element(tag), getElementById:element, addEventListener(){}};
  const accounts = [
    {id:'bravo.json', auth_index:'bravo', priority:20}, {id:'alpha-old-id', auth_index:'alpha', priority:20},
    {id:'delta.json', priority:10}, {id:'charlie.json', priority:10}, {id:'echo.json', priority:10},
  ];
  const board = {members:[{kind:'provider', priority:30}, ...accounts.map(a => {
    const name = a.id === 'alpha-old-id' ? 'alpha.json' : a.id;
    return {kind:'credential', id:name, name, key:'credential:'+name, priority:a.priority, weights:[{auth_index:a.auth_index}]};
  })]};
  const preferences = {preferences:{routing_card_order:{'credential:bravo.json':'1','credential:alpha.json':'2','credential:delta.json':'3'}}};
  const control = {snapshot:0, offline:false};
  const reads = [];
  const snapshot = () => ({server_time:'2026-01-01T00:00:00Z', state:'healthy', accounts, history:{epoch:'one', incidents:[], decisions:[{
    model:control.snapshot ? 'other-model' : 'first-model',
    decision:{auth_id:accounts[control.snapshot].id, candidates:[{
      auth_id:accounts[control.snapshot].id, priority:99, five_hour_reset:control.snapshot ? '2026-01-01T00:01:00Z' : '2026-01-01T04:00:00Z',
    }]},
  }]} });
  const context = {document, URL, AbortController, AbortSignal, location:{origin, pathname:consolePath+'/plugin-host/v0/resource/plugins/quota-first-router/monitor'},
    setTimeout:()=>1, clearTimeout(){}, localStorage:{getItem:()=>null}, matchMedia:()=>({addEventListener(){}}),
    async fetch(url,options) {
      const path = new URL(url,origin).pathname;
      reads.push(path);
      if (path.endsWith('/monitor')) return {ok:true,json:async()=>snapshot()};
      assert.equal(new URL(url).origin,origin);
      assert.equal(options.headers,undefined,'the CPA management key must not be sent to Oh My CPA');
      assert.equal(options.credentials,'same-origin');
      return {ok:!control.offline, status:control.offline?503:200, json:async()=>path.endsWith('/preferences')?preferences:board};
    },
  };
  context.window=context; context.parent=context;
  vm.runInNewContext(fs.readFileSync(require('node:path').join(__dirname,'../web/monitor.js'),'utf8'),context);
  element('key').value='fixture-key';element('login').handlers.submit({preventDefault(){}});
  const tick=()=>new Promise(resolve=>setImmediate(resolve));
  await tick();
  const list=element('accounts'), order=()=>list.findAll('article').map(card=>card.dataset.accountId);
  const selected=()=>list.findAll('article').filter(card=>card.dataset.selected).map(card=>card.dataset.accountId);
  assert.deepEqual(reads.slice(1),[consolePath+'/management/routing',consolePath+'/preferences']);
  assert.deepEqual(list.findAll('h2').map(heading=>heading.textContent),['Tier 2 (Priority 20)','Tier 3 (Priority 10)'],'full Routing board determines tier numbering');
  const expected=['bravo.json','alpha-old-id','delta.json','charlie.json','echo.json'];
  assert.deepEqual(order(),expected,'saved cards first, followed by unranked accounts alphabetically');
  assert.deepEqual(selected(),['bravo.json']);
  assert.equal(list.findAll('.badge')[0].textContent,'Selected');
  control.snapshot=1; control.offline=true;
  element('refresh').click();await tick();
  assert.deepEqual(order(),expected,'a new pick, changed reset and console outage must not reorder cards');
  assert.deepEqual(selected(),['alpha-old-id']);
  control.offline=false;
  preferences.preferences.routing_card_order['credential:alpha.json']='0';
  element('refresh').click();await tick();
  assert.deepEqual(order(),['alpha-old-id','bravo.json','delta.json','charlie.json','echo.json'],'refresh reads changes saved on the Routing page');
  console.log('PASS monitor: saved Routing order, stable tier numbers and Selected badge, and offline fallback');
}

main().then(routingOrder).catch(error=>{console.error(error);process.exitCode=1;});
