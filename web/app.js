'use strict';
const $ = id => document.getElementById(id);
const token = location.hash.slice(1) || sessionStorage.getItem('ep-token') || '';
if (location.hash) { sessionStorage.setItem('ep-token', token); history.replaceState(null, '', location.pathname + location.search); }
let busy = false, finishedID = 0, lastOutput = '', lastOutputAction = '', polling = false, exiting = false;
let toastTimer, browseMode = 'directory', browseCallback, browseData, selectedFile = '', browseVersion = 0;
const human = bytes => bytes >= 1073741824 ? (bytes / 1073741824).toFixed(2) + ' GB' : bytes >= 1048576 ? (bytes / 1048576).toFixed(2) + ' MB' : bytes >= 1024 ? (bytes / 1024).toFixed(1) + ' KB' : bytes + ' B';
const basename = p => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || 'project';
const parent = p => { const pos = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\')); return pos < 0 ? '' : p.slice(0, pos + 1); };
const join = (dir, name) => dir.replace(/[\\/]+$/, '') + (dir.includes('\\') || /^[A-Za-z]:/.test(dir) ? '\\' : '/') + name;
function toast(message) { clearTimeout(toastTimer); $('toast').textContent = message; $('toast').hidden = false; toastTimer = setTimeout(() => $('toast').hidden = true, 6500); }
async function api(path, data) {
 const response = await fetch('/api/' + path, {method: data === undefined ? 'GET' : 'POST', headers: {'X-EasyPackage-Token': token, ...(data === undefined ? {} : {'Content-Type': 'application/json'})}, ...(data === undefined ? {} : {body: JSON.stringify(data)})});
 if (!response.ok) { const text = await response.text(); let message = text; try { message = JSON.parse(text).error; } catch {} const error = new Error(message); error.status = response.status; throw error; }
 return response.json();
}
function nameFor(id, extension) {
 const raw = $(id).value.trim(); const name = extension ? raw.replace(/\.(zip|jpe?g)$/i, '') : raw;
 if (!name || /[\\/\x00-\x1f<>:"|?*]/.test(name) || name === '.' || name === '..' || /[. ]$/.test(name) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(name)) throw new Error('请输入有效的文件名，不包含路径分隔符或系统保留名称');
 return name + extension;
}
function settings() {
 const source = $('source').value.trim(), directory = $('directory').value.trim();
 if (!source || !directory) throw new Error('请先选择项目文件夹和保存目录');
 let maxBytes = 0;
 if ($('filter-large').checked) { const size = Number($('size').value), scaled = size * Number($('unit').value); maxBytes = Math.round(scaled); if (!Number.isFinite(scaled) || size <= 0 || !Number.isSafeInteger(maxBytes) || maxBytes < 1) throw new Error('请输入有效的正数大小，至少 1 字节'); }
 const cover = $('use-cover').checked ? $('cover').value.trim() : '';
 if ($('use-cover').checked && !cover) throw new Error('请选择 JPG 封面图片');
 const password = $('use-password').checked ? $('password').value : '';
 if ($('use-password').checked && !password) throw new Error('请输入压缩密码');
 return {source, output: join(directory, nameFor('filename', $('use-cover').checked ? '.jpg' : '.zip')), filterClass: $('filter-class').checked, maxBytes, cover, password, overwrite: false};
}
function syncSummary() {
 $('size-settings').hidden = !$('filter-large').checked; $('cover-settings').hidden = !$('use-cover').checked;
 $('password-settings').hidden = !$('use-password').checked;
 $('extension').textContent = $('use-cover').checked ? '.jpg' : '.zip';
 $('summary-format').textContent = ($('use-cover').checked ? 'JPG + ZIP' : 'ZIP') + ($('use-password').checked ? ' · 已加密' : ' · 无密码');
 $('summary-class').textContent = $('filter-class').checked ? '自动过滤' : '全部保留';
 $('summary-size').textContent = $('filter-large').checked ? $('size').value + ' ' + $('unit').selectedOptions[0].textContent : '不限';
 $('summary-output').textContent = ($('filename').value.replace(/\.(zip|jpe?g)$/i, '') || 'project') + $('extension').textContent;
 const restoreSource = $('restore-source').value.trim(), restoreDir = $('restore-directory').value.trim();
 $('restore-summary-file').textContent = restoreSource ? basename(restoreSource) : '尚未选择文件';
 $('restore-summary-file').title = restoreSource;
 $('restore-summary-target').textContent = restoreDir ? join(restoreDir, $('restore-filename').value.trim() || 'restored') : '请选择保存目录';
}
function passwordVisibility(inputID, visible) {
 const input = $(inputID), toggle = $(inputID + '-toggle');
 input.type = visible ? 'text' : 'password';
 toggle.textContent = visible ? '隐藏' : '显示';
 toggle.setAttribute('aria-pressed', String(visible));
 toggle.setAttribute('aria-label', (visible ? '隐藏' : '显示') + (inputID === 'password' ? '压缩密码' : '解压密码'));
}
function generatePassword() {
 // Rejection sampling avoids modulo bias; all entropy comes from Web Crypto.
 const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%*-_+';
 const ceiling = 256 - (256 % alphabet.length); let password = '';
 while (password.length < 20) {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  for (const byte of bytes) { if (byte < ceiling) password += alphabet[byte % alphabet.length]; if (password.length === 20) break; }
 }
 return password;
}
function showPage(page, push = true) {
 const restore = page === 'restore';
 clearTimeout(toastTimer); $('toast').hidden = true;
 if (restore && !$('restore-directory').value) $('restore-directory').value = $('directory').value;
 $('pack-page').hidden = restore; $('restore-page').hidden = !restore;
 for (const [id, active] of [['nav-pack', !restore], ['nav-restore', restore]]) {
  $(id).classList.toggle('active', active);
  if (active) $(id).setAttribute('aria-current', 'page'); else $(id).removeAttribute('aria-current');
 }
 $('page-name').textContent = restore ? 'JPG / ZIP 还原' : '项目打包';
 document.title = (restore ? 'JPG / ZIP 还原' : '项目打包') + ' · EasyPackage';
 passwordVisibility('password', false); passwordVisibility('restore-password', false);
 showReadyStatus(restore);
 syncSummary();
 if (push) {
  const url = new URL(location.href); if (restore) url.searchParams.set('page', 'restore'); else url.searchParams.delete('page');
  if (url.href !== location.href) history.pushState(null, '', url);
 }
 if (window.innerWidth <= 620) window.scrollTo({top: 0, behavior: 'instant'});
 else $('page-scroll').scrollTop = 0;
 updateSummaryPosition();
}
function updateSummaryPosition() {
 const main = document.querySelector('main');
 const scroll = $('page-scroll');
 const scrollbarWidth = Math.max(0, scroll.offsetWidth - scroll.clientWidth);
 const rightEdge = main.getBoundingClientRect().right - parseFloat(getComputedStyle(main).paddingRight) - scrollbarWidth;
 const rightInset = Math.max(0, document.documentElement.clientWidth - rightEdge);
 document.documentElement.style.setProperty('--summary-right', rightInset + 'px');
}
function showReadyStatus(restore) {
 $('status-title').textContent = restore ? '准备还原' : '准备就绪';
 $('percent').textContent = '等待任务'; $('progress').classList.remove('indeterminate'); $('progress').style.width = '0%';
 $('status-detail').textContent = restore ? '选择 JPG 或 ZIP 文件，开始还原。' : '选择一个项目，开始你的第一次打包。';
 $('status-detail').removeAttribute('title'); $('status-mark').textContent = '✓';
 $('cancel').hidden = true; $('show-output').hidden = true;
}
function setBusy(value) {
 busy = value; const locked = value || exiting;
 $('controls').disabled = locked; $('restore-controls').disabled = locked;
 $('start').disabled = locked; $('preview').disabled = locked; $('restore-start').disabled = locked;
 $('nav-pack').disabled = exiting; $('nav-restore').disabled = exiting;
 const restorePage = !$('restore-page').hidden;
 const outputMatchesPage = lastOutputAction && (lastOutputAction === 'restore') === restorePage;
 $('cancel').hidden = !value; $('cancel').disabled = false; $('show-output').hidden = value || !lastOutput || !outputMatchesPage; document.body.classList.toggle('busy', value);
}
async function start(action, options) {
 if (busy) return;
 try {
  const data = {...(options || settings()), action};
  setBusy(true);
  try { await api('start', data); }
  catch (error) { if (error.status === 409 && confirm('输出文件已存在，是否覆盖？\n\n' + data.output)) { data.overwrite = true; await api('start', data); } else { setBusy(false); if (error.status !== 409) throw error; return; } }
  lastOutput = ''; lastOutputAction = '';
  $('status-title').textContent = action === 'restore' ? '正在解压项目…' : '正在扫描项目…'; $('status-detail').textContent = '正在本机处理，请稍候。'; $('progress').classList.add('indeterminate'); $('percent').textContent = '处理中';
  await poll();
 } catch (error) { setBusy(false); toast(error.message); }
}
async function poll() {
 if (polling || exiting) return; polling = true;
 try {
  const state = await api('state');
  const badge = document.querySelector('.local-badge'); badge.classList.remove('disconnected'); badge.textContent = '● 本地连接';
  const restorePage = !$('restore-page').hidden;
  if (state.status === 'idle') { showReadyStatus(restorePage); return; }
  const running = state.status === 'running'; setBusy(running);
  const stateBelongsToPage = (state.action === 'restore') === restorePage;
  if (!stateBelongsToPage) {
   if (running) {
    $('status-title').textContent = state.action === 'restore' ? '还原任务处理中' : '打包任务处理中';
    $('percent').textContent = '处理中'; $('progress').classList.add('indeterminate');
    $('status-detail').textContent = '正在后台处理，请稍候。'; $('status-mark').textContent = '↗';
   } else showReadyStatus(restorePage);
   return;
  }
  $('status-title').textContent = state.phase;
  const plan = state.plan;
  const percentage = state.status === 'done' ? 100 : plan && plan.bytes > 0 ? Math.min(99, Math.floor(state.done * 100 / plan.bytes)) : 0;
  $('progress').classList.toggle('indeterminate', running && !plan); $('progress').style.width = percentage + '%'; $('percent').textContent = running ? (plan ? percentage + '%' : '处理中') : state.status === 'done' ? '已完成' : '已停止';
  const description = plan ? `${plan.files} 个文件 · ${human(plan.bytes)} · 过滤 ${plan.filtered} 个文件 / ${plan.excludedDirs || 0} 个目录 · 跳过链接 ${plan.links} 项` : '';
  $('status-detail').textContent = state.error || (running && state.current ? state.current : description || state.output || '临时文件已清理'); $('status-detail').title = $('status-detail').textContent;
  $('status-mark').textContent = running ? '↗' : state.status === 'error' ? '!' : state.status === 'cancelled' ? '−' : '✓';
  if (!running && state.id !== finishedID) {
   finishedID = state.id;
   if (state.status === 'error') toast(state.error);
   if (state.status === 'done' && state.preview && plan) showPreview(plan);
   if (state.status === 'done' && state.output) { lastOutput = state.output; lastOutputAction = state.action; $('show-output').hidden = false; toast('已保存：' + state.output); if (state.outputDirectory) { $('restore-password').value = ''; passwordVisibility('restore-password', false); } }
  }
 } catch (error) {
  const badge = document.querySelector('.local-badge'); badge.textContent = '连接已断开'; badge.classList.add('disconnected');
  $('status-title').textContent = '无法连接本地程序'; $('status-detail').textContent = error.status === 401 ? '请使用程序本次启动时打开的页面。' : '请重新启动 EasyPackage；运行中的任务状态暂时无法获取。';
 } finally { polling = false; }
}
function showPreview(plan) {
 $('preview-stats').textContent = `${plan.files} 个文件 / ${human(plan.bytes)} / 过滤 ${plan.filtered} 个文件、${plan.excludedDirs || 0} 个目录及内部内容 / 跳过 ${plan.links} 个链接或特殊文件`;
 const fragment = document.createDocumentFragment();
 for (const item of plan.items) { const row = document.createElement('div'); row.className = 'preview-row'; const name = document.createElement('span'); name.textContent = item.name; const size = document.createElement('small'); size.textContent = item.directory ? '文件夹' : human(item.size); row.append(name, size); fragment.append(row); }
 $('preview-list').replaceChildren(fragment); if (!$('preview-dialog').open) $('preview-dialog').showModal();
}
async function browse(mode, initial, callback) {
 browseMode = mode; browseCallback = callback; selectedFile = ''; browseData = null;
 $('browser-title').textContent = mode === 'jpg' ? '选择 JPG 图片' : mode === 'archive' ? '选择 JPG / ZIP' : '选择文件夹'; $('browser-select').textContent = mode === 'jpg' ? '使用这张图片' : mode === 'archive' ? '选择此文件' : '选择此文件夹';
 $('browser-search').value = ''; $('file-list').replaceChildren(); $('browser-select').disabled = true;
 $('browser-dialog').showModal(); await loadDirectory(initial || '');
}
async function loadDirectory(path) {
 const version = ++browseVersion; $('browser-message').textContent = '正在读取文件夹…'; $('browser-select').disabled = true;
 $('browser-path').disabled = true; $('browser-go').disabled = true; $('browser-up').disabled = true;
 try {
  const data = await api('browse?path=' + encodeURIComponent(path)); if (version !== browseVersion) return;
  browseData = data; selectedFile = ''; $('browser-path').value = data.path; $('browser-selection').textContent = data.path;
  $('browser-search').value = ''; $('browser-up').disabled = data.path === data.parent;
  $('browser-shortcuts').replaceChildren();
  for (const [title, shortcuts] of [['常用位置', [...data.shortcuts, ...data.roots.map(p => ({label: p, path: p}))]], ['最近使用', data.recent.map(p => ({label: basename(p), path: p}))]]) {
   const group = document.createElement('div'); group.className = 'shortcut-group'; const heading = document.createElement('span'); heading.textContent = title; group.append(heading);
   if (!shortcuts.length) { const empty = document.createElement('small'); empty.textContent = '选择文件夹后显示，最多 5 个'; group.append(empty); }
   for (const shortcut of shortcuts) { const b = document.createElement('button'); b.textContent = shortcut.label; b.title = shortcut.path; b.addEventListener('click', () => loadDirectory(shortcut.path)); group.append(b); }
   $('browser-shortcuts').append(group);
  }
  $('browser-select').disabled = browseMode === 'jpg' || browseMode === 'archive'; renderFiles();
 } catch (error) { if (version !== browseVersion) return; browseData = null; $('file-list').replaceChildren(); $('browser-message').textContent = error.message; }
 finally { if (version === browseVersion) { $('browser-path').disabled = false; $('browser-go').disabled = false; } }
}
function renderFiles() {
 if (!browseData) return;
 const search = $('browser-search').value.toLocaleLowerCase();
 const matching = browseData.entries.filter(e => e.name.toLocaleLowerCase().includes(search) && (e.directory || browseMode === 'jpg' && /\.jpe?g$/i.test(e.name) || browseMode === 'archive' && /\.(jpe?g|zip)$/i.test(e.name)));
 const visible = matching.slice(0, 1500), fragment = document.createDocumentFragment();
 for (const entry of visible) {
  const row = document.createElement('button'); row.className = 'file-row' + (selectedFile === entry.path ? ' selected' : '');
  const icon = document.createElement('span'); icon.textContent = entry.directory ? '▱' : '▧';
  const name = document.createElement('span'); name.className = 'file-name'; name.textContent = entry.name;
  const tail = document.createElement('small'); tail.textContent = entry.directory ? '进入 ›' : human(entry.size); row.append(icon, name, tail);
  row.addEventListener('click', () => { if (entry.directory) loadDirectory(entry.path); else { selectedFile = entry.path; $('browser-selection').textContent = entry.path; $('browser-select').disabled = false; renderFiles(); } });
  if (!entry.directory) row.addEventListener('dblclick', chooseCurrent);
  fragment.append(row);
 }
 if (!visible.length) { const empty = document.createElement('p'); empty.className = 'empty-state'; empty.textContent = browseMode === 'jpg' ? '当前目录没有匹配的图片或子文件夹' : '当前目录没有匹配的子文件夹，可直接选择此目录'; fragment.append(empty); }
 $('file-list').replaceChildren(fragment);
 $('browser-message').textContent = browseData.truncated ? '目录超过 10,000 项，当前列表不完整；可在地址栏输入准确路径。' : matching.length > 1500 ? '匹配项超过 1,500 个，请搜索缩小范围。' : '点击文件夹进入；支持在上方直接输入完整目录路径。';
}
async function chooseCurrent() { const isFile = browseMode === 'jpg' || browseMode === 'archive'; const value = isFile ? selectedFile : browseData?.path; if (!value) return; $('browser-dialog').close(); browseCallback(value); syncSummary(); try { await api('recent', {path: isFile ? parent(value) : value}); } catch (e) { toast('目录已选择，但历史记录保存失败：' + e.message); } }
function sourceChosen(path) { $('source').value = path; if (!$('directory').value) $('directory').value = parent(path) || path; if ($('filename').value === 'project' || !$('filename').value) $('filename').value = basename(path); syncSummary(); }
$('choose-source').addEventListener('click', () => browse('directory', $('source').value, sourceChosen));
$('source').addEventListener('change', () => sourceChosen($('source').value.trim()));
$('choose-directory').addEventListener('click', () => browse('directory', $('directory').value, p => $('directory').value = p));
$('choose-cover').addEventListener('click', () => browse('jpg', parent($('cover').value) || $('source').value, p => $('cover').value = p));
$('browser-go').addEventListener('click', () => loadDirectory($('browser-path').value)); $('browser-path').addEventListener('keydown', e => { if (e.key === 'Enter') loadDirectory(e.target.value); });
$('browser-up').addEventListener('click', () => loadDirectory(browseData ? browseData.parent : parent($('browser-path').value)));
$('browser-search').addEventListener('input', renderFiles); $('browser-select').addEventListener('click', chooseCurrent);
$('browser-close').addEventListener('click', () => $('browser-dialog').close()); $('browser-dialog').addEventListener('close', () => browseVersion++);
$('preview-close').addEventListener('click', () => $('preview-dialog').close());
$('nav-pack').addEventListener('click', () => showPage('pack'));
document.querySelector('.brand').addEventListener('click', e => { e.preventDefault(); showPage('pack'); });
$('nav-restore').addEventListener('click', () => showPage('restore'));
window.addEventListener('popstate', () => showPage(new URLSearchParams(location.search).get('page'), false));
for (const id of ['password', 'restore-password']) $(id + '-toggle').addEventListener('click', () => passwordVisibility(id, $(id).type === 'password'));
$('password-generate').addEventListener('click', () => { try { $('password').value = generatePassword(); passwordVisibility('password', false); toast('已生成 20 位随机密码，可点击「显示」查看并保存。'); } catch { toast('随机生成失败，请手动输入密码。'); } });
$('use-password').addEventListener('change', () => passwordVisibility('password', false));
$('choose-restore-source').addEventListener('click', () => browse('archive', parent($('restore-source').value) || $('directory').value, p => { $('restore-source').value = p; $('restore-filename').value = basename(p).replace(/\.(jpe?g|zip)$/i, '') + '-restored'; if (!$('restore-directory').value) $('restore-directory').value = parent(p); }));
$('choose-restore-directory').addEventListener('click', () => browse('directory', $('restore-directory').value, p => $('restore-directory').value = p));
$('restore-start').addEventListener('click', () => { try { const source = $('restore-source').value.trim(), directory = $('restore-directory').value.trim(); if (!source || !directory) throw new Error('请选择 JPG / ZIP 文件和还原位置'); start('restore', {source, output: join(directory, nameFor('restore-filename', '')), password: $('restore-password').value, overwrite: false}); } catch (e) { toast(e.message); } });
$('start').addEventListener('click', () => start('pack')); $('preview').addEventListener('click', () => start('preview'));
$('cancel').addEventListener('click', async () => { try { $('cancel').disabled = true; await api('cancel', {}); $('status-title').textContent = '正在取消…'; } catch (e) { toast(e.message); } });
$('show-output').addEventListener('click', async () => { try { await api('reveal', {}); } catch (e) { toast(e.message); } });
$('quit').addEventListener('click', async () => { if (busy && !confirm('退出会取消正在执行的任务，是否继续？')) return; try { await api('quit', {}); exiting = true; setBusy(false); $('controls').disabled = true; $('start').disabled = true; $('preview').disabled = true; $('nav-restore').disabled = true; $('show-output').hidden = true; $('status-title').textContent = '程序正在退出'; $('status-detail').textContent = 'EasyPackage 窗口即将关闭。'; document.querySelector('.local-badge').textContent = '正在退出'; toast('EasyPackage 正在退出…'); } catch (e) { toast(e.message); } });
for (const id of ['filter-class', 'filter-large', 'size', 'unit', 'filename', 'use-cover', 'use-password', 'restore-source', 'restore-directory', 'restore-filename']) $(id).addEventListener('input', syncSummary);
$('quit-footer').addEventListener('click', () => $('quit').click());
document.querySelectorAll('.summary-column').forEach(column => column.addEventListener('wheel', event => {
 if (window.innerWidth > 940) {
  event.preventDefault();
  $('page-scroll').scrollTop += event.deltaY;
 }
}, {passive: false}));
window.addEventListener('resize', updateSummaryPosition, {passive: true});
new ResizeObserver(updateSummaryPosition).observe($('page-scroll'));
updateSummaryPosition();
showPage(new URLSearchParams(location.search).get('page'), false); poll(); setInterval(poll, 650);
