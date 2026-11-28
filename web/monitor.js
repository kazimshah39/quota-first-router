'use strict';
(() => {
 const $ = id => document.getElementById(id);
 const endpoint = '/v0/management/plugins/quota-first-router/monitor';
 const hiddenAbort = Symbol('hidden tab');
 const pluginFrameSuffix = '/plugin-host/v0/resource/plugins/quota-first-router/monitor';
 const consolePath = location.pathname?.endsWith('/api/v1' + pluginFrameSuffix)
  ? location.pathname.slice(0, -pluginFrameSuffix.length) : '';
 let key = '', data = null, timer = null, controller = null, failures = 0, selectedModel = '', manualKey = false, allActivity = false, generation = 0;
 let routing = null, routingReadAt = null;
 let quotaFilter = 'all';
 const filterNames = {all:'All', available:'Available', exhausted:'Exhausted', free:'Free'};
 const activityLimit = 6;
 const systemTheme = matchMedia('(prefers-color-scheme: dark)');
 const reasons = {ready:'Quota usable',exhausted:'Quota exhausted',stale:'Quota observation is stale',unknown:'Quota unknown',disabled:'Account disabled',free_plan_excluded:'Free plan excluded',identity_changed:'Credential changed; waiting for quota',reset_pending_refresh:'Reset passed; waiting for fresh quota',earliest_five_hour_reset:'Earliest five-hour reset in highest usable tier',usable_unknown_reset:'Usable quota; reset time unknown',tier_quota_unknown_delegate:'Unknown quota in highest usable tier; delegated to CPA',no_usable_quota:'No observed usable quota',no_known_usable_quota:'No known usable quota',provider_not_exclusively_supported:'Not exclusively Codex; delegated to CPA'};
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
 function timestamp(value) {
  return value && !String(value).startsWith('0001') ? Date.parse(value) : NaN;
 }
 function showTiming(next, reset) {
  let last = -Infinity, upcoming = Infinity;
  for (const a of next.accounts) {
   const read = timestamp(a.quota?.read_at), attempt = timestamp(a.next_refresh);
   if (Number.isFinite(read)) last = Math.max(last, read);
   if (Number.isFinite(attempt)) upcoming = Math.min(upcoming, attempt);
  }
  const elapsed = Number.isFinite(last) ? Math.max(0, Math.floor((Date.parse(next.server_time) - last) / 1000)) + 's ago' : 'Not yet';
  const attempt = Number.isFinite(upcoming) ? stamp(upcoming) : 'Not scheduled';
  const status = $('connection');
  status.className = 'good';
  status.hidden = false;
  status.textContent = (reset ? 'History reset · ' : '') + 'Last read: ' + elapsed + ' · Next attempt: ' + attempt;
 }
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
  generation++; controller?.abort(); key=''; data=null; routing=null; routingReadAt=null; failures=0; selectedModel=''; manualKey=false; allActivity=false;
  clearTimeout(timer);
  for (const id of ['private','toolbar','signout','health','model-filter','activity-more']) $(id).hidden = true;
  quotaFilter = 'all';
  for (const id of ['accounts','activity','health','coverage','history-note','activity-count','filter-status']) $(id).replaceChildren();
  updateFilterCounts({all:0, available:0, exhausted:0, free:0});
  $('routing-activity').open = false;
  $('model').replaceChildren(node('option','Latest model'));
  $('model').firstChild.value=''; $('search').value=''; $('login').hidden=false;
  $('connection').textContent=message; $('connection').className=message ? 'warning' : ''; $('connection').hidden=!message;
 }
 async function readRouting(signal) {
  // Only Oh My CPA's same-origin plugin host exposes its saved arrangement. The
  // console session authorizes these GETs; never send the CPA management key.
  const options = {signal: AbortSignal.any([signal, AbortSignal.timeout(2500)]), cache:'no-store', credentials:'same-origin', redirect:'error'};
  async function read(path) {
   const url = new URL(location.origin);
   url.pathname = consolePath + path;
   const response = await fetch(url.href, options);
   if (response.status === 401 || response.status === 403) return null;
   if (!response.ok) throw new Error('Routing order not available');
   return response.json();
  }
  const [board, preferences] = await Promise.all([read('/management/routing'), read('/preferences')]);
  if (!Array.isArray(board?.members)) return null;
  const priorities = new Set(), members = new Map(), membersByIndex = new Map(), ranks = new Map();
  for (const [position, member] of board.members.entries()) {
   if (!Number.isInteger(member?.priority)) continue;
   priorities.add(member.priority);
   if (member.kind !== 'credential' || typeof member.id !== 'string' || typeof member.key !== 'string' || typeof member.name !== 'string') continue;
   const {key, name, priority} = member;
   const card = {key, name, priority, position};
   members.set(member.id, card);
   const index = member.weights?.[0]?.auth_index;
   if (typeof index === 'string' && index) membersByIndex.set(index, card);
  }
  const saved = preferences?.preferences?.routing_card_order;
  for (const [key, rank] of Object.entries(saved ?? {})) {
   if (typeof rank === 'string' && /^\d+$/.test(rank) && Number.isSafeInteger(Number(rank))) ranks.set(key, Number(rank));
  }
  return {priorities, members, membersByIndex, ranks};
 }
 function schedule(){clearTimeout(timer);if(!document.hidden&&key)timer=setTimeout(load,Math.min(60000,5000*2**Math.min(failures,4)));}
 async function load(){if(controller||!key||document.hidden)return;clearTimeout(timer);controller=new AbortController();const current=controller,epoch=generation,timeout=setTimeout(()=>current.abort(),10000);$('refresh').disabled=true;
  try {
   const response = await fetch(endpoint,{headers:{Authorization:'Bearer '+key},signal:current.signal,cache:'no-store',credentials:'same-origin'});
   if (epoch!==generation || current.signal.aborted) return;
   if (response.status===401 || response.status===403) { disconnect('Management access was rejected. Connect again to continue.'); return; }
   if (!response.ok) throw new Error(response.status===404?'Monitor unavailable. The plugin may be disabled or unloaded; check Plugin management.':`Monitor request failed (HTTP ${response.status}).`);
   const next = await response.json();
   if (epoch!==generation || current.signal.aborted) return;
   if (!next.history) throw new Error('Monitor is stopped or unavailable. Check Plugin management.');
   if (consolePath && (routingReadAt===null || Date.now()-routingReadAt>=30000)) {
    routingReadAt = Date.now();
    try {
     const arrangement = await readRouting(current.signal);
     if (epoch===generation && !current.signal.aborted) routing = arrangement;
    } catch { /* Keep the last saved order on temporary console failures. */ }
   }
   if (epoch!==generation || current.signal.aborted) return;
   const reset = data && data.history.epoch!==next.history.epoch;
   data=next;failures=0;render();showTiming(next,reset);$('private').hidden=false;$('toolbar').hidden=false;$('signout').hidden=!manualKey;$('login').hidden=true;
  }
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
 function quotaAvailability(a, model, now) {
  if (a.availability === 'exhausted' || a.availability === 'stale') return a.availability;
  const windows = a.quota?.model_windows?.[modelKey(model)] || [];
  const elapsed = w => w.resets_at && Date.parse(w.resets_at) <= now;
  if (windows.some(w => w.utilization >= 100 && !elapsed(w))) return 'exhausted';
  if (windows.some(elapsed)) return 'reset_pending_refresh';
  return a.availability;
 }
 function quotaGroup(a, model, now) {
  if (a.free_plan_excluded === true) return 'free';
  const status = quotaAvailability(a, model, now);
  return status === 'ready' ? 'available' : status === 'exhausted' ? 'exhausted' : '';
 }
 function updateFilterCounts(counts) {
  for (const [filter, title] of Object.entries(filterNames)) {
   const button = $('filter-' + filter);
   const label = title + ' (' + counts[filter] + ')';
   if (button.getAttribute('aria-label') !== label) {
    button.replaceChildren(node('span', title), node('span', counts[filter], 'filter-count'));
    button.setAttribute('aria-label', label);
   }
   button.setAttribute('aria-pressed', String(quotaFilter === filter));
  }
 }
 function accountGroups(accounts) {
  const groups = new Map();
  const priorities = new Set(routing?.priorities);
  for (const account of accounts) {
   const member = routing?.membersByIndex.get(account.auth_index) ?? routing?.members.get(account.id);
   const priority = member?.priority ?? (Number.isInteger(account.priority) ? account.priority : null);
   if (priority !== null) priorities.add(priority);
   if (!groups.has(priority)) groups.set(priority, []);
   groups.get(priority).push({account, member});
  }
  const tiers = [...priorities].sort((a,b) => b-a);
  if (groups.has(null)) tiers.push(null);
  return tiers.flatMap((priority, index) => {
   const group = groups.get(priority);
   if (!group) return [];
   const title = priority === null ? 'Tier unknown' : `Tier ${index + 1}`;
   group.sort((a,b) => {
    const rank = item => routing?.ranks.get(item.member?.key) ?? Infinity;
    const name = item => item.member?.name ?? accountTitle(item.account);
    const position = item => item.member?.position ?? Infinity;
    return rank(a) - rank(b) || name(a).localeCompare(name(b)) || position(a) - position(b) || a.account.id.localeCompare(b.account.id);
   });
   return [{title, priority, accounts: group.map(item => item.account)}];
  });
 }
 function renderAccount(a, note, record, now) {
  const account = node('article', undefined, 'account');
  account.dataset.accountId = a.id;
  const heading = node('header', undefined, 'account-head');
  const identity = node('div');
  const title = node('h3', accountTitle(a));
  const availability = quotaAvailability(a, record?.model, now);
  const exhausted = availability === 'exhausted';
  const meta = [];
  const plan = planNames.get(a.quota?.plan_type);
  if (plan) meta.push(plan);
  if (a.free_plan_excluded === true) meta.push('Excluded from automatic routing and warmup');
  if (!exhausted && availability !== 'ready' && availability !== 'unknown') meta.push(reason(availability));
  identity.append(title);
  if (meta.length) identity.append(node('p', meta.join(' · '), 'account-meta'));
  heading.append(identity);
  const markers = node('div', undefined, 'account-markers');
  if (note && record.decision.auth_id === a.id && a.free_plan_excluded !== true) {
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
  quota.append(quotaReading(a.quota?.seven_day, now, a.quota?.five_hour_absent ? 'Monthly allowance' : 'Weekly', stale));
  account.append(heading, quota);
  const blocking = w => !stale && w.utilization >= 100 && (!w.resets_at || Date.parse(w.resets_at) > now);
  const model = modelKey(record?.model);
  if (model && (a.quota?.model_windows?.[model] || []).some(blocking)) account.append(node('p', record.model + ' quota limit reached', 'account-notice warning'));
  if (a.last_error) account.append(node('p', 'Quota check failed: ' + a.last_error, 'account-notice bad'));
  if (a.warmup_status === 'refused' || a.warmup_status === 'unknown') {
   account.append(node('p', a.warmup_status === 'refused' ? 'Warmup request refused' : 'Warmup outcome unknown', 'account-notice warning'));
  }
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
  const search = $('search').value.trim().toLowerCase();
  const counts = {all:0, available:0, exhausted:0, free:0};
  const list = $('accounts');
  let shown = 0;
  const sections = accountGroups(data.accounts).flatMap(({title, priority, accounts}) => {
   const cards = [];
   for (const a of accounts) {
    if (!a.id.toLowerCase().includes(search) && !a.email?.toLowerCase().includes(search) && !accountTitle(a).toLowerCase().includes(search)) continue;
    const group = quotaGroup(a, record?.model, now);
    counts.all++;
    if (group) counts[group]++;
    if (quotaFilter !== 'all' && quotaFilter !== group) continue;
    cards.push(renderAccount(a, notes.get(a.id), record, now));
   }
   if (!cards.length) return [];
   shown += cards.length;
   const section = node('section', undefined, 'account-tier');
   const grid = node('div', undefined, 'account-grid');
   grid.append(...cards);
   const heading = node('h2', title);
   if (priority !== null) heading.append(node('span', ' (Priority ' + priority + ')', 'tier-priority'));
   section.append(heading, grid);
   return [section];
  });
  updateFilterCounts(counts);
  const filterStatus = shown + (shown === 1 ? ' account shown' : ' accounts shown');
  if ($('filter-status').textContent !== filterStatus) $('filter-status').textContent = filterStatus;
  list.replaceChildren(...sections);
  if (!shown) empty(list, search ? 'No accounts match this search and filter.' : {
   all:'No Codex accounts discovered.', available:'No accounts with usable quota.',
   exhausted:'No accounts with confirmed quota exhaustion.', free:'No confirmed Free accounts.',
  }[quotaFilter]);
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
 const help = $('monitor-help');
 $('open-help').addEventListener('click',()=>{
  if (help.open) return;
  help.showModal();
  $('help-content').scrollTop = 0;
 });
 help.addEventListener('close',()=>$('open-help').focus());
 $('refresh').addEventListener('click',()=>{routingReadAt=null;load();});
 $('signout').addEventListener('click',()=>{controller?.abort();disconnect('Key forgotten for this page. Connect again to continue.');$('key').focus();});
 $('search').addEventListener('input',render);$('model').addEventListener('change',()=>{selectedModel=$('model').value;render();});
 for (const filter of Object.keys(filterNames)) $('filter-' + filter).addEventListener('click',()=>{
  if (quotaFilter !== filter) { quotaFilter = filter; render(); }
 });
 $('activity-more').addEventListener('click',()=>{allActivity=!allActivity;renderActivity();});
 systemTheme.addEventListener('change',setTheme);
 document.addEventListener('visibilitychange',()=>{clearTimeout(timer);if(document.hidden)controller?.abort(hiddenAbort);else {routingReadAt=null;load();}});
 try{if(parent!==window)new MutationObserver(setTheme).observe(parent.document.documentElement,{attributes:true,attributeFilter:['data-theme','data-theme-mode']});}catch{}
 setTheme();key=inheritedKey();if(key)load();else disconnect('');
})();
