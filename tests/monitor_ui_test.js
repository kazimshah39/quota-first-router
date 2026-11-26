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
  addEventListener(type, fn) { this.handlers[type] = fn; }
  setAttribute(k,v) { this.attributes[k] = v; }
  focus() {}
  click() { this.handlers.click?.({preventDefault(){}}); }
}
async function main() {
  const ids = new Map(); const element = id => { if (!ids.has(id)) ids.set(id,new Element()); return ids.get(id); };
  let requests = [], observations = 0, writes = 0, color = 'dark';
  const document = { hidden:false,documentElement:new Element(),createElement:tag=>new Element(tag),getElementById:element,addEventListener(type,fn){this[type]=fn;} };
  const context = { document, URL, Uint8Array, TextEncoder, TextDecoder, atob, AbortController, Date, JSON, console,
    localStorage:{getItem(){return null;},setItem(){writes++;}}, location:{origin:'http://fixture',host:'fixture'},
    MutationObserver:class {observe(){observations++;}}, matchMedia:()=>({matches:color==='dark'}),
    setTimeout:()=>1,clearTimeout(){},
    fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));},
  };
  context.window=context; context.parent=context;
  vm.runInNewContext(fs.readFileSync(require('node:path').join(__dirname,'../web/monitor.js'),'utf8'),context);
  assert.equal(observations,0,'standalone must not observe itself');
  assert.equal(document.documentElement.dataset.theme,'dark');
  element('theme').click(); element('theme').click(); element('theme').click();
  color='light';element('theme').click();element('theme').click();element('theme').click();
  assert.equal(document.documentElement.dataset.theme,'light','auto follows system, not previous self theme');
  const submit=key=>{element('key').value=key;element('login').handlers.submit({preventDefault(){}});};
  const tick=()=>new Promise(resolve=>setImmediate(resolve));
  submit('secret');assert.equal(requests.length,1);assert.equal(element('key').value,'');
  element('refresh').click();assert.equal(requests.length,1,'one request in flight');
  element('signout').click();assert.equal(requests[0].options.signal.aborted,true);
  const snapshot={version:'0.2.0',server_time:new Date().toISOString(),mode:'active',state:'healthy',accounts:[],total_accounts:0,fresh_accounts:0,picks:0,routed:0,poll_interval:'1m0s',history:{epoch:'one',started_at:new Date().toISOString(),decisions:[],incidents:[],decision_evictions:0,incident_evictions:0,candidate_truncations:0}};
  requests[0].resolve({ok:true,status:200,json:async()=>snapshot});await tick();
  assert.equal(element('private').hidden,true,'late response must not restore logged-out diagnostics');
  assert.equal(element('history-note').textContent,'');
  submit('secret');requests[1].resolve({ok:true,status:200,json:async()=>snapshot});await tick();
  assert.equal(element('private').hidden,false);
  element('refresh').click();requests[2].resolve({ok:false,status:403});await tick();
  assert.equal(element('private').hidden,true);assert.equal(element('history-note').textContent,'');assert.equal(element('model').textContent,'Latest selection');
  assert.equal(writes,0,'must not persist entered key');
  submit('secret');document.hidden=true;document.visibilitychange();assert.equal(requests[3].options.signal.aborted,true,'hidden page aborts');
  requests[3].resolve({ok:true,status:200,json:async()=>snapshot});await tick();assert.equal(element('private').hidden,true);
  console.log('PASS monitor controller: standalone theme, one-inflight, logout race, forbidden clearing, hidden abort, memory-only key');
}
main().catch(error=>{console.error(error);process.exitCode=1;});
