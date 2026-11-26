'use strict';
(() => {
 const $ = id => document.getElementById(id);
 const endpoint = '/v0/management/plugins/quota-first-router/monitor';
 let key = '', data = null, timer = null, controller = null, paused = false, failures = 0, selectedModel = '', theme = 'auto', generation = 0;
 const reasons = {ready:'Quota usable',exhausted:'Quota exhausted',stale:'Quota observation is stale',unknown:'Quota unknown',disabled:'Account disabled',identity_changed:'Credential changed; waiting for quota',reset_pending_refresh:'Reset passed; waiting for fresh quota',earliest_five_hour_reset:'Earliest five-hour reset in highest usable tier',usable_unknown_reset:'Usable quota; reset time unknown',tier_quota_unknown_delegate:'Unknown quota in highest usable tier; delegated to CPA',no_usable_quota:'No observed usable quota',no_known_usable_quota:'No known usable quota',provider_not_exclusively_supported:'Not exclusively Codex; delegated to CPA'};
 const reason = code => reasons[code] || code || 'Not offered / eligibility unknown';
 function node(tag, text, cls) {const n = document.createElement(tag);if(text!==undefined)n.textContent=String(text);if(cls)n.className=cls;return n;}
 function empty(el,text){el.replaceChildren(node('p',text,'muted'));}
 const stamp = value => value && !String(value).startsWith('0001') ? new Date(value).toLocaleString() : 'Not yet';
 function countdown(value,now){if(!value)return 'Reset unknown';const seconds=Math.ceil((Date.parse(value)-now)/1000);if(seconds<=0)return 'Reset passed; refresh needed';return `Resets in ${Math.floor(seconds/3600)}h ${Math.floor(seconds%3600/60)}m`;}
 function age(value,now){return value?`${Math.max(0,Math.floor((now-Date.parse(value))/1000))}s ago`:'No successful refresh';}
 function inheritedKey(){try{let raw=localStorage.getItem('cli-proxy-auth');if(!raw)return '';if(raw.startsWith('enc::v2::')){const bytes=Uint8Array.from(atob(raw.slice(9)),c=>c.charCodeAt(0));const salt=new TextEncoder().encode('cli-proxy-api-webui::secure-storage|'+location.host);for(let i=0;i<bytes.length;i++)bytes[i]^=salt[i%salt.length];raw=new TextDecoder().decode(bytes);}const state=JSON.parse(raw).state;const base=new URL(state.apiBase||location.origin,location.origin);return base.origin===location.origin&&typeof state.managementKey==='string'?state.managementKey:'';}catch{return '';}}
 function setTheme(){let value=theme;try{if(value==='auto'&&parent!==window)value=parent.document.documentElement.dataset.theme||'';}catch{}if(!['light','dark'].includes(value))value=matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light';if(document.documentElement.dataset.theme!==value)document.documentElement.dataset.theme=value;}
 function disconnect(message){generation++;controller?.abort();key='';data=null;failures=0;selectedModel='';clearTimeout(timer);$('private').hidden=true;for(const id of ['summary','accounts','latest','incidents','decisions','health','health-note','order-note','coverage','history-note','activity-count'])$(id).replaceChildren();$('model').replaceChildren(node('option','Latest selection'));$('model').firstChild.value='';$('search').value='';$('login').hidden=false;$('connection').textContent=message;$('connection').className='warning';}
 function schedule(){clearTimeout(timer);if(!paused&&!document.hidden&&key)timer=setTimeout(load,Math.min(60000,5000*2**Math.min(failures,4)));}
 async function load(){if(controller||!key||document.hidden)return;clearTimeout(timer);controller=new AbortController();const current=controller,epoch=generation,timeout=setTimeout(()=>current.abort(),10000);$('refresh').disabled=true;
  try{const response=await fetch(endpoint,{headers:{Authorization:'Bearer '+key},signal:controller.signal,cache:'no-store',credentials:'same-origin'});if(epoch!==generation||current.signal.aborted)return;if(response.status===401||response.status===403){disconnect('Management access was rejected. Connect again to continue.');return;}if(!response.ok)throw new Error(response.status===404?'Monitor unavailable. The plugin may be disabled or unloaded; check Plugin management.':`Monitor request failed (HTTP ${response.status}).`);const next=await response.json();if(epoch!==generation||current.signal.aborted)return;if(!next.history)throw new Error('Monitor is stopped or unavailable. Check Plugin management.');const reset=data&&data.history.epoch!==next.history.epoch;data=next;failures=0;render();$('connection').className='good';$('connection').textContent=(reset?'History reset after restart or reconfiguration. ':'')+`Updated ${new Date().toLocaleTimeString()}. ${paused?'Updates paused.':'Updates every 5 seconds while visible.'}`;$('private').hidden=false;$('login').hidden=true;}
  catch(error){if(epoch!==generation||current.signal.aborted&&(document.hidden||paused))return;failures++;$('connection').className='warning';$('connection').textContent=(data?'Displayed data is stale. ':'')+(error.name==='AbortError'?'Monitor request timed out.':error.message)+' Use Refresh now to retry.';}
  finally{clearTimeout(timeout);if(controller===current)controller=null;$('refresh').disabled=false;schedule();}
 }
 function quotaReading(q, now, title, stale, unavailable='Not reported') {
  const reading = node('div', undefined, 'quota-reading');
  const heading = node('div', undefined, 'quota-line');
  heading.append(node('span', title, 'quota-name'));
  reading.append(heading);
  if (!Number.isFinite(q?.utilization)) {
   reading.append(node('span', unavailable, 'quota-caption'));
   return reading;
  }
  const remaining = Math.min(100, Math.max(0, 100 - q.utilization));
  const display = remaining > 0 && remaining < .1 ? '<0.1' : Math.round(remaining * 10) / 10;
  const text = `${display}% left`;
  heading.append(node('strong', text, 'quota-value'));
  const bar = node('progress', undefined, 'quota-bar');
  bar.max = 100;
  bar.value = remaining;
  bar.dataset.tone = stale ? 'cached' : remaining >= 70 ? 'good' : remaining >= 25 ? 'low' : 'critical';
  bar.setAttribute('aria-label', `${title} quota remaining`);
  bar.setAttribute('aria-valuetext', stale ? `${text} at last check (stale)` : text);
  reading.append(bar, node('span', countdown(q.resets_at, now), 'quota-caption'));
  return reading;
 }
 function accountTitle(id) {
  // Strip only known Codex filename prefixes and only when the rest is an email.
  const email = id.replace(/\.json$/i, '').replace(/^codex-(?:9router-)?[0-9a-f]{8}-/i, '');
  return /^[\w.!#$%&'*+\/=?^_`{|}~-]+@(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9]+$/i.test(email) ? email : id;
 }
 function renderAccount(a, note, record, now, expanded) {
  const account = node('article', undefined, 'account');
  account.dataset.accountId = a.id;
  const heading = node('header', undefined, 'account-head');
  const identity = node('div');
  const title = node('h3', accountTitle(a.id));
  const state = a.availability === 'ready' ? '' : ` · ${reason(a.availability)}`;
  identity.append(title, node('p', (note ? `Tier ${note.priority}` : 'Tier unknown') + state, 'account-meta'));
  heading.append(identity);
  if (note && record.decision.auth_id === a.id) {
   account.dataset.selected = 'true';
   heading.append(node('span', record.mode === 'active' ? 'Selected' : 'Would select', 'badge'));
  }
  const quota = node('div', undefined, 'quota-grid');
  const stale = ['stale', 'reset_pending_refresh'].includes(a.availability);
  if (!a.quota?.five_hour_absent) quota.append(quotaReading(a.quota?.five_hour, now, '5 hours', stale));
  quota.append(quotaReading(a.quota?.seven_day, now, a.quota?.five_hour_absent ? 'Monthly allowance' : 'Long window', stale));
  const extra = node('details', undefined, 'account-extra');
  extra.open = expanded;
  extra.append(node('summary', a.last_error ? 'Polling error · details' : 'Details', a.last_error ? 'warning' : ''));
  if (note?.reason && note.reason !== 'ready' && note.reason !== a.availability) {
   extra.append(node('p', `At last decision: ${reason(note.reason)}`, 'detail'));
  }
  extra.append(node('p', `Last read: ${age(a.quota?.read_at, now)} · Next attempt: ${stamp(a.next_refresh)}`, 'detail'));
  if (a.last_error) extra.append(node('p', `${a.last_error} · ${a.consecutive_failures} consecutive failures`, 'bad'));
  if (a.quota?.five_hour_absent) extra.append(node('p', 'No five-hour window; ranks after known resets within the same tier.', 'detail'));
  if (a.quota_details_truncated) extra.append(node('p', `${a.quota_details_truncated} quota details omitted by safety limits.`, 'warning'));
  const windows = node('div', undefined, 'quota-grid');
  for (const window of a.quota?.other_windows || []) windows.append(quotaReading(window, now, 'Other limit', stale));
  const modelFilter = selectedModel.trim().toLowerCase().replace(/^codex\//, '');
  for (const [model, list] of Object.entries(a.quota?.model_windows || {})) {
   if (modelFilter && model !== modelFilter) continue;
   for (const window of list) windows.append(quotaReading(window, now, model, stale));
  }
  if (windows.children.length) extra.append(windows);
  account.append(heading, quota, extra);
  return account;
 }
 function render(){if(!data)return;setTheme();const now=Date.parse(data.server_time),h=data.history;const availableModels=[...new Set(h.decisions.map(r=>r.model).filter(Boolean))];const select=$('model');select.replaceChildren(node('option','Latest selection'));select.firstChild.value='';for(const model of availableModels){const option=node('option',model);option.value=model;select.append(option);}if(!availableModels.includes(selectedModel))selectedModel='';select.value=selectedModel;
  const record=h.decisions.find(r=>!selectedModel||r.model===selectedModel),d=record?.decision;const notes=new Map((d?.candidates||[]).map(c=>[c.auth_id,c]));
  $('health').textContent={healthy:'Quota checks healthy',starting:'Waiting for quota',no_accounts:'No eligible accounts',degraded:'Needs attention',failed:'Worker failed'}[data.state]||data.state;
  $('health').className=data.state==='healthy'?'good':data.state==='failed'?'bad':'warning';$('health-note').textContent=data.worker_failed?'The polling worker failed. Check host logs and reconfigure or reload the plugin.':data.list_error?'Credential roster could not be read.':data.state==='degraded'?'Check the quota warnings below.':'';
  $('health-note').hidden = !$('health-note').textContent;
  $('summary').replaceChildren();for(const [label,value] of [['Mode / version',`${data.mode} · v${data.version}`],['Fresh quota',`${data.fresh_accounts} / ${data.total_accounts}`]]){const pair=node('div');pair.append(node('dt',label),node('dd',value));$('summary').append(pair);}
  $('order-note').textContent = record ? `Tier order at ${stamp(record.at)} · ${record.model || 'Unknown model'}` : 'Tier order appears after the first request.';
  $('coverage').hidden = !(data.accounts_truncated || record?.truncated);
  $('coverage').textContent = `${data.accounts_truncated||0} accounts and ${record?.truncated||0} candidate details omitted by safety limits.`;
  const search=$('search').value.toLowerCase();const accounts=[...data.accounts].filter(a=>a.id.toLowerCase().includes(search));accounts.sort((a,b)=>{const x=notes.get(a.id),y=notes.get(b.id);if(!x&&!y)return a.id.localeCompare(b.id);if(!x)return 1;if(!y)return -1;return y.priority-x.priority||(x.five_hour_reset?Date.parse(x.five_hour_reset):Infinity)-(y.five_hour_reset?Date.parse(y.five_hour_reset):Infinity)||a.id.localeCompare(b.id);});
  const list = $('accounts');
  const expanded = new Set([...list.children].filter(card => card.querySelector('details')?.open).map(card => card.dataset.accountId));
  const focusedSummary = document.activeElement;
  const focusedCard = focusedSummary?.tagName === 'SUMMARY' && focusedSummary.closest('.account');
  const focusedID = focusedCard ? focusedCard.dataset.accountId : null;
  list.replaceChildren(...accounts.map(a => renderAccount(a, notes.get(a.id), record, now, expanded.has(a.id))));
  if (focusedID !== null) [...list.children].find(card => card.dataset.accountId === focusedID)?.querySelector('summary').focus({preventScroll:true});
  if (!accounts.length) empty(list, search ? 'No matching accounts.' : 'No eligible Codex accounts have been discovered.');
  $('activity-count').textContent = h.incidents.length ? `· ${h.incidents.length} ${h.incidents.length === 1 ? 'incident' : 'incidents'}` : '';
  $('latest').textContent=record?`${record.mode==='shadow'?'Shadow proposal':'Scheduler choice'}: ${d.auth_id||'delegated to CPA'} · ${record.model} · ${record.stream?'streaming':'non-streaming'} · ${stamp(record.at)} · ${reason(d.reason)}`:'No scheduler decisions yet. Send ordinary traffic through CPA to observe selections.';
  $('incidents').replaceChildren();for(const i of h.incidents.filter(i=>!search||i.account?.toLowerCase().includes(search))){const li=node('li');li.append(node('time',stamp(i.last_at)),node('strong',i.code==='recovered'?'Recovered':i.code,i.code==='recovered'?'good':'warning'),node('span',`${i.account||'Plugin'} · ${i.kind} · ${i.count} occurrence(s); first ${stamp(i.at)}`,'detail'));$('incidents').append(li);}if(!$('incidents').children.length)empty($('incidents'),'No recorded problems or recoveries in this history.');
  $('decisions').replaceChildren();for(const r of h.decisions.filter(r=>(!selectedModel||r.model===selectedModel)&&(!search||(r.decision.auth_id||'').toLowerCase().includes(search)))){const li=node('li');li.append(node('time',`${stamp(r.at)} · #${r.sequence}`),node('strong',r.decision.auth_id||'Delegated to CPA'),node('span',`${r.model} · ${r.mode} · ${r.stream?'stream':'non-stream'} · ${reason(r.decision.reason)}`,'detail'));$('decisions').append(li);}if(!$('decisions').children.length)empty($('decisions'),'No matching scheduler decisions.');
  const omittedHistory = [];
  if (h.decision_evictions) omittedHistory.push(`${h.decision_evictions} older decisions removed`);
  if (h.incident_evictions) omittedHistory.push(`${h.incident_evictions} older incidents removed`);
  if (h.candidate_truncations) omittedHistory.push(`${h.candidate_truncations} account details omitted`);
  $('history-note').textContent = omittedHistory.join(' · ');
  $('history-note').hidden = omittedHistory.length === 0;
 }
 $('login').addEventListener('submit',event=>{event.preventDefault();generation++;key=$('key').value.trim();$('key').value='';failures=0;load();});
 $('refresh').addEventListener('click',()=>{if(key)load();else{$('login').hidden=false;$('key').focus();}});
 $('pause').addEventListener('click',()=>{paused=!paused;$('pause').textContent=paused?'Resume updates':'Pause updates';$('pause').setAttribute('aria-pressed',String(paused));clearTimeout(timer);if(paused){controller?.abort();$('connection').textContent='Updates paused. Displayed data is a snapshot; Refresh now still works.';}else load();});
 $('signout').addEventListener('click',()=>{controller?.abort();disconnect('Key forgotten for this page. Connect again to continue.');$('key').focus();});
 $('search').addEventListener('input',render);$('model').addEventListener('change',()=>{selectedModel=$('model').value;render();});
 $('theme').addEventListener('click',()=>{theme=theme==='auto'?'dark':theme==='dark'?'light':'auto';$('theme').textContent='Theme: '+theme;setTheme();});
 document.addEventListener('visibilitychange',()=>{clearTimeout(timer);if(document.hidden)controller?.abort();else if(!paused)load();});
 try{if(parent!==window)new MutationObserver(setTheme).observe(parent.document.documentElement,{attributes:true,attributeFilter:['data-theme','class','style']});}catch{}
 setTheme();key=inheritedKey();if(key)load();else disconnect('Connect with your management key. No diagnostic data is public.');
})();
