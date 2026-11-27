'use strict';
(() => {
 const $ = id => document.getElementById(id);
 const endpoint = '/v0/management/plugins/quota-first-router/monitor';
 const hiddenAbort = Symbol('hidden tab');
 let key = '', data = null, timer = null, controller = null, failures = 0, selectedModel = '', manualKey = false, allActivity = false, generation = 0;
 const activityLimit = 6;
 const systemTheme = matchMedia('(prefers-color-scheme: dark)');
 const reasons = {ready:'Quota usable',exhausted:'Quota exhausted',stale:'Quota observation is stale',unknown:'Quota unknown',disabled:'Account disabled',identity_changed:'Credential changed; waiting for quota',reset_pending_refresh:'Reset passed; waiting for fresh quota',earliest_five_hour_reset:'Earliest five-hour reset in highest usable tier',usable_unknown_reset:'Usable quota; reset time unknown',tier_quota_unknown_delegate:'Unknown quota in highest usable tier; delegated to CPA',no_usable_quota:'No observed usable quota',no_known_usable_quota:'No known usable quota',provider_not_exclusively_supported:'Not exclusively Codex; delegated to CPA'};
 const reason = code => reasons[code] || code || 'Not offered / eligibility unknown';
 function node(tag, text, cls) {const n = document.createElement(tag);if(text!==undefined)n.textContent=String(text);if(cls)n.className=cls;return n;}
 function empty(el,text){el.replaceChildren(node('p',text,'muted'));}
 const stamp = value => value && !String(value).startsWith('0001') ? new Date(value).toLocaleString() : 'Not yet';
 function countdown(value, now) {
  const diff = Date.parse(value) - now;
  if (!Number.isFinite(diff)) return 'Reset unknown';
  if (diff <= 0) return 'Waiting for reset check';
  const minutes = Math.ceil(diff / 60000);
  const hours = Math.floor(minutes / 60);
  if (!hours) return `${minutes}m left`;
  if (hours < 24) return `${hours}h${minutes % 60 ? ` ${minutes % 60}m` : ''} left`;
  return `${Math.floor(hours / 24)}d${hours % 24 ? ` ${hours % 24}h` : ''} left`;
 }
 function age(value,now){return value?`${Math.max(0,Math.floor((now-Date.parse(value))/1000))}s ago`:'No successful refresh';}
 function inheritedKey(){try{let raw=localStorage.getItem('cli-proxy-auth');if(!raw)return '';if(raw.startsWith('enc::v2::')){const bytes=Uint8Array.from(atob(raw.slice(9)),c=>c.charCodeAt(0));const salt=new TextEncoder().encode('cli-proxy-api-webui::secure-storage|'+location.host);for(let i=0;i<bytes.length;i++)bytes[i]^=salt[i%salt.length];raw=new TextDecoder().decode(bytes);}const state=JSON.parse(raw).state;const base=new URL(state.apiBase||location.origin,location.origin);return base.origin===location.origin&&typeof state.managementKey==='string'?state.managementKey:'';}catch{return '';}}
 function setTheme() {
  let value;
  try {
   if (parent !== window) {
    const root = parent.document.documentElement;
    value = root.dataset.theme || root.dataset.themeMode;
   }
  } catch { /* A standalone page just uses the browser preference. */ }
  if (value !== 'light' && value !== 'dark') value = systemTheme.matches ? 'dark' : 'light';
  if (document.documentElement.dataset.theme !== value) document.documentElement.dataset.theme = value;
 }
 function disconnect(message) {
  generation++; controller?.abort(); key=''; data=null; failures=0; selectedModel=''; manualKey=false; allActivity=false;
  clearTimeout(timer);
  for (const id of ['private','toolbar','signout','health','model-filter','activity-more']) $(id).hidden = true;
  for (const id of ['accounts','activity','health','coverage','history-note','activity-count']) $(id).replaceChildren();
  $('routing-activity').open = false;
  $('model').replaceChildren(node('option','Latest model'));
  $('model').firstChild.value=''; $('search').value=''; $('login').hidden=false;
  $('connection').textContent=message; $('connection').className=message ? 'warning' : ''; $('connection').hidden=!message;
 }
 function schedule(){clearTimeout(timer);if(!document.hidden&&key)timer=setTimeout(load,Math.min(60000,5000*2**Math.min(failures,4)));}
 async function load(){if(controller||!key||document.hidden)return;clearTimeout(timer);controller=new AbortController();const current=controller,epoch=generation,timeout=setTimeout(()=>current.abort(),10000);$('refresh').disabled=true;
  try{const response=await fetch(endpoint,{headers:{Authorization:'Bearer '+key},signal:controller.signal,cache:'no-store',credentials:'same-origin'});if(epoch!==generation||current.signal.aborted)return;if(response.status===401||response.status===403){disconnect('Management access was rejected. Connect again to continue.');return;}if(!response.ok)throw new Error(response.status===404?'Monitor unavailable. The plugin may be disabled or unloaded; check Plugin management.':`Monitor request failed (HTTP ${response.status}).`);const next=await response.json();if(epoch!==generation||current.signal.aborted)return;if(!next.history)throw new Error('Monitor is stopped or unavailable. Check Plugin management.');const reset=data&&data.history.epoch!==next.history.epoch;data=next;failures=0;render();$('connection').className='good';$('connection').hidden=false;$('connection').textContent=(reset?'History reset · ':'')+`Updated ${new Date().toLocaleTimeString()}`;$('private').hidden=false;$('toolbar').hidden=false;$('signout').hidden=!manualKey;$('login').hidden=true;}
  catch(error){if(epoch!==generation||document.hidden||current.signal.reason===hiddenAbort)return;failures++;$('connection').className='warning';$('connection').hidden=false;$('connection').textContent=(data?'Displayed data is stale. ':'')+(error.name==='AbortError'?'Monitor request timed out.':error.message)+' Use Refresh to retry.';}
  finally{clearTimeout(timeout);if(controller===current)controller=null;$('refresh').disabled=false;if(current.signal.reason===hiddenAbort&&!document.hidden)load();else schedule();}
 }
 function quotaReading(q, now, title, stale, idle=false) {
  const reading = node('div', undefined, 'quota-reading');
  const heading = node('div', undefined, 'quota-line');
  heading.append(node('span', title, 'quota-name'));
  reading.append(heading);
  if (!Number.isFinite(q?.utilization)) {
   reading.append(node('span', 'Not reported', 'quota-caption'));
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
  const reset = node('span', idle && !stale ? 'Timer not started' : countdown(q.resets_at, now), 'quota-caption');
  if ((!idle || stale) && Number.isFinite(Date.parse(q.resets_at))) reset.title = `Reset: ${stamp(q.resets_at)}`;
  reading.append(bar, reset);
  return reading;
 }
 const displayEmail = email => email.replace(/@googlemail\.com$/i, '@gmail.com');
 const planNames = new Map([['free','Free'],['go','Go'],['plus','Plus'],['pro','Pro'],['team','Team'],['business','Business'],['enterprise','Enterprise'],['edu','Edu']]);
 function accountTitle(a) {
  if (a.email) return displayEmail(a.email);
  // Older snapshots may only have the file name, including Codex's plan suffix.
  const id = a.id;
  const stem = id.replace(/\.json$/i, '');
  const prefix = /^codex-(?:(?:9router-)?[0-9a-f]{8}-)?/i;
  const candidate = prefix.test(stem) ? stem.replace(prefix, '').replace(/-(?:free|plus|pro|team|business|enterprise|edu)$/i, '') : stem;
  return /^[\w.!#$%&'*+\/=?^_`{|}~-]+@(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9]+$/i.test(candidate) ? displayEmail(candidate) : id;
 }
 const modelKey = model => (model || '').trim().toLowerCase().replace(/^codex\//, '');
 function exhaustedFor(a, model, now) {
  if (a.availability === 'exhausted') return true;
  if (a.availability === 'stale') return false;
  const key = modelKey(model);
  return Boolean(key && a.quota?.model_windows?.[key]?.some(w => w.utilization >= 100 && (!w.resets_at || Date.parse(w.resets_at) > now)));
 }
 function renderAccount(a, note, record, now, expanded) {
  const account = node('article', undefined, 'account');
  account.dataset.accountId = a.id;
  const heading = node('header', undefined, 'account-head');
  const identity = node('div');
  const title = node('h3', accountTitle(a));
  const exhausted = exhaustedFor(a, record?.model, now);
  const priority = note?.priority ?? a.priority;
  const meta = [];
  const plan = planNames.get(a.quota?.plan_type);
  if (plan) meta.push(plan);
  if (!exhausted && a.availability !== 'ready' && a.availability !== 'unknown') meta.push(reason(a.availability));
  identity.append(title);
  if (meta.length) identity.append(node('p', meta.join(' · '), 'account-meta'));
  heading.append(identity);
  const markers = node('div', undefined, 'account-markers');
  if (note && record.decision.auth_id === a.id) {
   account.dataset.selected = 'true';
   markers.append(node('span', 'Selected', 'badge'));
  }
  if (exhausted) {
   account.dataset.exhausted = 'true';
   const warning = node('span', '!', 'quota-alert');
   warning.title = 'Quota exhausted';
   warning.setAttribute('role', 'img');
   warning.setAttribute('aria-label', warning.title);
   markers.append(warning);
  }
  if (markers.children.length) heading.append(markers);
  const quota = node('div', undefined, 'quota-grid');
  const stale = ['stale', 'reset_pending_refresh'].includes(a.availability);
  if (!a.quota?.five_hour_absent) quota.append(quotaReading(a.quota?.five_hour, now, '5 hours', stale, a.five_hour_idle === true));
  quota.append(quotaReading(a.quota?.seven_day, now, a.quota?.five_hour_absent ? 'Monthly allowance' : 'Long window', stale));
  const extra = node('details', undefined, 'account-extra');
  extra.open = expanded;
  extra.append(node('summary', a.last_error ? 'Polling error · details' : 'Details', a.last_error ? 'warning' : ''));
  if (Number.isInteger(priority)) extra.append(node('p', `Routing priority: ${priority}`, 'detail'));
  const warmup = new Map([
   ['pending', 'Waiting to send'], ['sent', 'Sent; checking the timer'], ['confirmed', 'Window confirmed'],
   ['billing_unverified', 'Skipped: subscription-only use not confirmed'], ['quota_blocked', 'Skipped: quota not ready'],
   ['credential_unavailable', 'Skipped: account unavailable or cooling down'], ['model_unavailable', 'Skipped: model unavailable'],
   ['cpa_unavailable', 'Skipped: CPA unavailable'], ['already_attempted', 'Already attempted this window'],
   ['state_unavailable', 'Paused: state unavailable'], ['refused', 'Request refused'], ['unknown', 'Outcome unknown; not retried'],
  ]).get(a.warmup_status);
  if (warmup) extra.append(node('p', `Warmup: ${warmup}`, 'detail'));
  if (note?.reason && note.reason !== 'ready' && note.reason !== a.availability && !(exhausted && note.reason === 'exhausted')) {
   extra.append(node('p', `At last decision: ${reason(note.reason)}`, 'detail'));
  }
  extra.append(node('p', `Last read: ${age(a.quota?.read_at, now)} · Next attempt: ${stamp(a.next_refresh)}`, 'detail'));
  if (a.last_error) extra.append(node('p', `${a.last_error} · ${a.consecutive_failures} consecutive failures`, 'bad'));
  if (a.quota?.five_hour_absent) extra.append(node('p', 'No five-hour window; ranks after known resets within the same tier.', 'detail'));
  if (a.quota_details_truncated) extra.append(node('p', `${a.quota_details_truncated} quota details omitted by safety limits.`, 'warning'));
  const windows = node('div', undefined, 'quota-grid');
  for (const window of a.quota?.other_windows || []) windows.append(quotaReading(window, now, 'Other limit', stale));
  const modelFilter = modelKey(selectedModel);
  for (const [model, list] of Object.entries(a.quota?.model_windows || {})) {
   if (modelFilter && model !== modelFilter) continue;
   for (const window of list) windows.append(quotaReading(window, now, model, stale));
  }
  if (windows.children.length) extra.append(windows);
  account.append(heading, quota, extra);
  return account;
 }
 function render() {
  if (!data) return;
  const now = Date.parse(data.server_time), h = data.history;
  const availableModels = [...new Set(h.decisions.map(r => r.model).filter(Boolean))];
  const select = $('model');
  $('model-filter').hidden = availableModels.length < 2;
  if (availableModels.length < 2 || !availableModels.includes(selectedModel)) selectedModel = '';
  select.replaceChildren(node('option', 'Latest model'));
  select.firstChild.value = '';
  for (const model of availableModels) {
   const option = node('option', model);
   option.value = model;
   select.append(option);
  }
  select.value = selectedModel;
  const record = h.decisions.find(r => !selectedModel || r.model === selectedModel);
  const notes = new Map((record?.decision?.candidates || []).map(c => [c.auth_id,c]));
  const health = $('health');
  health.textContent = data.list_error ? 'Account list unavailable' : ({
   healthy: `${data.fresh_accounts}/${data.total_accounts} fresh`,
   starting: 'Checking quota…', no_accounts: '', degraded: 'Needs attention', failed: 'Quota checks failed',
  }[data.state] || '');
  if (data.warmup_error) health.textContent = [health.textContent, 'Warmup paused'].filter(Boolean).join(' · ');
  health.className = data.state === 'failed' ? 'bad' : data.state === 'healthy' && !data.warmup_error ? 'good' : 'warning';
  health.hidden = !health.textContent;
  $('coverage').hidden = !(data.accounts_truncated || record?.truncated);
  $('coverage').textContent = `${data.accounts_truncated||0} accounts and ${record?.truncated||0} candidate details omitted by safety limits.`;
  const search=$('search').value.toLowerCase();const accounts=data.accounts.filter(a=>a.id.toLowerCase().includes(search)||a.email?.toLowerCase().includes(search)||accountTitle(a).toLowerCase().includes(search));accounts.sort((a,b)=>{const x=notes.get(a.id),y=notes.get(b.id);if(!x&&!y)return a.id.localeCompare(b.id);if(!x)return 1;if(!y)return -1;return y.priority-x.priority||(x.five_hour_reset?Date.parse(x.five_hour_reset):Infinity)-(y.five_hour_reset?Date.parse(y.five_hour_reset):Infinity)||a.id.localeCompare(b.id);});
  const list = $('accounts');
  const expanded = new Set([...list.children].filter(card => card.querySelector('details')?.open).map(card => card.dataset.accountId));
  const focusedSummary = document.activeElement;
  const focusedCard = focusedSummary?.tagName === 'SUMMARY' && focusedSummary.closest('.account');
  const focusedID = focusedCard ? focusedCard.dataset.accountId : null;
  list.replaceChildren(...accounts.map(a => renderAccount(a, notes.get(a.id), record, now, expanded.has(a.id))));
  if (focusedID !== null) [...list.children].find(card => card.dataset.accountId === focusedID)?.querySelector('summary').focus({preventScroll:true});
  if (!accounts.length) empty(list, search ? 'No matching accounts.' : 'No eligible Codex accounts have been discovered.');
  renderActivity();
 }
 function renderActivity() {
  if (!data) return;
  const h = data.history;
  const accounts = new Map(data.accounts.map(a => [a.id, a]));
  const labelFor = id => id ? accountTitle(accounts.get(id) || {id}) : '';
  const events = h.decisions.filter(r => !selectedModel || r.model === selectedModel).map(r => ({
   kind: r.decision.auth_id ? 'selected' : 'delegated',
   title: r.decision.auth_id ? `Selected ${labelFor(r.decision.auth_id)}` : 'CPA decided',
   detail: [r.model, reason(r.decision.reason)].filter(Boolean).join(' · '),
   time: r.at, sequence: r.sequence, accountId: r.decision.auth_id || '', count: 1,
  }));
  for (const i of h.incidents) {
   const recovered = i.code === 'recovered';
   const name = labelFor(i.account);
   const subject = i.kind === 'quota' ? 'Quota check' : i.kind === 'roster' ? 'Account list' : 'Quota worker';
   events.push({
    kind: recovered ? 'recovered' : 'issue',
    title: `${subject} ${recovered ? 'recovered' : 'failed'}`,
    detail: [name, i.count > 1 ? `×${i.count}` : '', recovered ? '' : i.code].filter(Boolean).join(' · '),
    time: i.last_at || i.at, sequence: i.sequence,
   });
  }
  events.sort((a,b) => Date.parse(b.time) - Date.parse(a.time) || b.sequence - a.sequence);
  const items = [];
  for (const event of events) {
   const last = items.at(-1);
   if (event.count && last?.kind === event.kind && last.accountId === event.accountId && last.detail === event.detail) last.count++;
   else items.push(event);
  }
  const issues = h.incidents.filter(i => i.code !== 'recovered').length;
  $('activity-count').textContent = issues ? `· ${issues} ${issues === 1 ? 'failure' : 'failures'} in history` : '';
  $('activity-empty').hidden = items.length > 0;
  $('activity').replaceChildren(...items.slice(0, allActivity ? items.length : activityLimit).map(item => {
   const row = node('li', undefined, 'activity-row');
   row.dataset.kind = item.kind;
   const dot = node('span', undefined, 'activity-dot');
   dot.setAttribute('aria-hidden', 'true');
   const label = node('div', undefined, 'activity-label');
   label.append(node('strong', item.title));
   if (item.count > 1) label.append(node('span', ` ×${item.count}`, 'detail'));
   if (item.detail) label.append(node('p', item.detail, 'detail'));
   const date = new Date(item.time);
   const time = node('time', Number.isFinite(date.getTime()) ? date.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}) : '—');
   time.dateTime = item.time || '';
   time.title = stamp(item.time);
   row.append(dot, label, time);
   return row;
  }));
  const more = $('activity-more');
  more.hidden = items.length <= activityLimit;
  more.textContent = allActivity ? 'Show less' : 'Show more';
  more.setAttribute('aria-expanded', String(allActivity));
  const older = (h.decision_evictions || 0) + (h.incident_evictions || 0);
  $('history-note').textContent = older ? `${older} older events removed` : '';
  $('history-note').hidden = !older;
 }
 $('login').addEventListener('submit',event=>{event.preventDefault();generation++;key=$('key').value.trim();manualKey=Boolean(key);$('key').value='';failures=0;load();});
 $('refresh').addEventListener('click',load);
 $('signout').addEventListener('click',()=>{controller?.abort();disconnect('Key forgotten for this page. Connect again to continue.');$('key').focus();});
 $('search').addEventListener('input',render);$('model').addEventListener('change',()=>{selectedModel=$('model').value;render();});
 $('activity-more').addEventListener('click',()=>{allActivity=!allActivity;renderActivity();});
 systemTheme.addEventListener('change',setTheme);
 document.addEventListener('visibilitychange',()=>{clearTimeout(timer);if(document.hidden)controller?.abort(hiddenAbort);else load();});
 try{if(parent!==window)new MutationObserver(setTheme).observe(parent.document.documentElement,{attributes:true,attributeFilter:['data-theme','data-theme-mode']});}catch{}
 setTheme();key=inheritedKey();if(key)load();else disconnect('');
})();
