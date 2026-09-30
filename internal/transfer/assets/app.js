const token = new URLSearchParams(location.search).get('t') || '';
const $ = (id) => document.getElementById(id);
let currentReview = null;
let reviewedInput = null;
let preparedId = '';
let uploadId = '';

async function api(route, body) {
  let response;
  try {
    response = await fetch(`${route}?t=${encodeURIComponent(token)}`, {
      method: body === undefined ? 'GET' : 'POST',
      headers: body === undefined ? {} : {'Content-Type': 'application/json', 'X-KH3-Token': token},
      body: body === undefined ? undefined : JSON.stringify(body)
    });
  } catch {
    throw new Error('No se pudo conectar con el asistente local. Comprueba que kh3save.exe sigue abierto. Si lo cerraste, vuelve a ejecutar Iniciar transferencia.bat y usa la página nueva que se abra.');
  }
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `La operación falló (${response.status}).`);
  return data;
}
function showError(error) {
  const box = $('message'); box.textContent = error.message || String(error); box.className = 'message error'; box.hidden = false;
  box.scrollIntoView({behavior:'smooth', block:'center'});
}
function clearError() { $('message').hidden = true; }
function setStep(n) {
  document.querySelectorAll('.step').forEach((el) => {
    const no = Number(el.dataset.step); el.classList.toggle('active', no === n); el.classList.toggle('done', no < n);
  });
}
function requestData() { return {source:$('source').value.trim(), uploadId, destination:$('destination').value.trim(), account:$('account').value.trim()}; }
function prettySize(n) { return n >= 1_000_000 ? `${(n/1_000_000).toFixed(1)} MB` : `${Math.ceil(n/1024)} KB`; }

async function discover() {
  try {
    const data = await api('/api/discover');
    if (!data.folders?.length) return;
    const box = $('epic-detected'); box.hidden = false;
    box.innerHTML = `<strong>Carpeta de Epic detectada</strong><button type="button" id="use-detected">${escapeHTML(data.folders[0].path)}</button>`;
    $('use-detected').addEventListener('click', () => { clearUploadMode(); $('source').value = data.folders[0].path; clearError(); });
  } catch (error) { showError(error); }
}
function escapeHTML(text) { return String(text).replace(/[&<>"']/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch])); }

async function browse(kind) {
  clearError();
  try {
    const data = await api('/api/browse', {kind});
    if (kind === 'epic') clearUploadMode();
    $(kind === 'epic' ? 'source' : 'destination').value = data.path;
    if (kind === 'steam') $('account').focus();
  } catch (error) { if (!error.message.includes('cancelada')) showError(error); }
}

function clearUploadMode() {
  const previous = uploadId;
  uploadId = '';
  if (previous) api('/api/release', {uploadId:previous}).catch(() => {});
  $('source-files').value = '';
  $('selected-files').hidden = true;
  $('selected-files').innerHTML = '';
}

async function uploadFiles(fileList) {
  const files = Array.from(fileList || []);
  if (!files.length) return;
  clearError();
  const previous = uploadId;
  uploadId = '';
  if (previous) await api('/api/release', {uploadId:previous}).catch(() => {});
  $('source').value = '';
  const list = $('selected-files');
  list.hidden = false;
  list.innerHTML = `<span class="upload-status"><span class="spinner"></span> Revisando ${files.length} archivo${files.length === 1 ? '' : 's'} en este equipo…</span>`;
  const form = new FormData();
  for (const file of files) form.append('files', file, file.name);
  try {
    let response;
    try {
      response = await fetch(`/api/upload?t=${encodeURIComponent(token)}`, {method:'POST', headers:{'X-KH3-Token':token}, body:form});
    } catch {
      throw new Error('No se pudo conectar con el asistente local. Comprueba que kh3save.exe sigue abierto. Si lo cerraste, vuelve a ejecutar Iniciar transferencia.bat y usa la página nueva que se abra.');
    }
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || 'No se pudieron cargar los archivos seleccionados.');
    uploadId = data.uploadId;
    list.innerHTML = `<div class="file-list-head"><strong>${data.files.length} archivos seleccionados y validados</strong><button type="button" id="clear-files" aria-label="Quitar archivos">Quitar</button></div><div class="file-chips">${data.files.map((file) => `<span class="file-chip"><span class="file-dot"></span><code>${escapeHTML(file.name)}</code><small>${prettySize(file.size)}</small></span>`).join('')}</div><p class="upload-status">Archivos transferidos únicamente al asistente local para preparar la conversión.</p>`;
    $('clear-files').addEventListener('click', clearUploadMode);
  } catch (error) {
    clearUploadMode();
    showError(error);
  }
}

async function review() {
  clearError();
  $('review').disabled = true; $('review').textContent = 'Validando archivos…';
  $('review-result').hidden = true; $('result-card').hidden = true; $('done-card').hidden = true;
  try {
    const data = await api('/api/review', requestData());
    currentReview = data;
    $('account').value = data.account;
    $('detected-label').hidden = false;
    reviewedInput = requestData();
    const rows = data.files.map((f) => `<tr><td><code>${escapeHTML(f.name)}</code></td><td>${prettySize(f.size)}</td><td class="platform">Epic Games Store</td><td class="${data.replacements.includes(f.name)?'replace':''}">${data.replacements.includes(f.name)?'Se reemplazará':'Se creará'}</td></tr>`).join('');
    const collision = data.replacements.length ? `<label class="overwrite"><input type="checkbox" id="overwrite"><span>Entiendo que estos archivos existentes de Steam se reemplazarán. Antes se guardará una copia visible de cada uno.</span></label>` : '';
    $('review-result').innerHTML = `<div class="table-wrap"><table><thead><tr><th>Archivo</th><th>Tamaño</th><th>Origen validado</th><th>Destino</th></tr></thead><tbody>${rows}</tbody></table></div><p class="target-line"><strong>Destino:</strong> <code>${escapeHTML(data.destination)}</code><br><strong>SteamID64:</strong> <code>${escapeHTML(data.account)}</code></p>${collision}<button class="primary" id="prepare">Convertir y validar en carpeta temporal <span>→</span></button>`;
    $('review-result').hidden = false;
    $('prepare').addEventListener('click', prepare);
    setStep(3);
    $('review-result').scrollIntoView({behavior:'smooth', block:'nearest'});
  } catch (error) { showError(error); }
  finally { $('review').disabled = false; $('review').innerHTML = 'Revisar archivos y destino <span>→</span>'; }
}

async function prepare() {
  clearError();
  if (JSON.stringify(requestData()) !== JSON.stringify(reviewedInput)) { showError(new Error('Cambiaste el origen, el destino o el SteamID64. Revisa otra vez antes de convertir.')); return; }
  const overwrite = $('overwrite')?.checked || false;
  if (currentReview.replacements.length && !overwrite) { showError(new Error('Confirma que quieres reemplazar los archivos indicados para continuar.')); return; }
  const button = $('prepare'); button.disabled = true; button.textContent = 'Convirtiendo y comprobando…';
  try {
    const data = await api('/api/prepare', {...requestData(), overwrite, expectedReplacements:currentReview.replacements});
    preparedId = data.id;
    $('prepared-summary').innerHTML = `<p class="target-line"><strong>${data.files.length} archivos listos</strong><br>${data.files.map(escapeHTML).join(', ')}<br><strong>Se instalarán en:</strong><br><code>${escapeHTML(data.destination)}</code><br><strong>SteamID64 validado:</strong> <code>${escapeHTML(data.account)}</code></p><p class="backup-line">Los originales de Epic se copiarán a una carpeta visible de respaldo. También se copiarán los archivos de Steam que vayan a reemplazarse.</p>`;
    $('confirm-install').checked = false; $('install').disabled = true; $('result-card').hidden = false; setStep(4);
    $('result-card').scrollIntoView({behavior:'smooth', block:'start'});
  } catch (error) { showError(error); }
  finally { button.disabled = false; button.innerHTML = 'Convertir y validar en carpeta temporal <span>→</span>'; }
}

async function install() {
  clearError(); $('install').disabled = true; $('install').textContent = 'Instalando…';
  try {
    const data = await api('/api/install', {id:preparedId, confirm:true});
    $('done-text').innerHTML = `Los archivos quedaron en <code>${escapeHTML(data.destination)}</code>.<br>Las copias de seguridad están en <code>${escapeHTML(data.backup)}</code>.<br>Archivos instalados: ${data.files.map(escapeHTML).join(', ')}.`;
    $('done-card').hidden = false; $('result-card').hidden = true; $('done-card').scrollIntoView({behavior:'smooth', block:'start'});
  } catch (error) { showError(error); $('install').disabled = false; $('install').innerHTML = 'Instalar guardados en Steam <span>→</span>'; }
}

$('pick-source').addEventListener('click', () => browse('epic'));
$('pick-destination').addEventListener('click', () => browse('steam'));
$('pick-files').addEventListener('click', () => $('source-files').click());
$('source-files').addEventListener('change', (event) => uploadFiles(event.target.files));
$('drop-zone').addEventListener('click', (event) => { if (!event.target.closest('button')) $('source-files').click(); });
$('drop-zone').addEventListener('dragover', (event) => { event.preventDefault(); $('drop-zone').classList.add('drag-over'); });
$('drop-zone').addEventListener('dragleave', () => $('drop-zone').classList.remove('drag-over'));
$('drop-zone').addEventListener('drop', (event) => { event.preventDefault(); $('drop-zone').classList.remove('drag-over'); uploadFiles(event.dataTransfer.files); });
$('source').addEventListener('input', () => { if ($('source').value.trim()) clearUploadMode(); });
$('review').addEventListener('click', review);
$('confirm-install').addEventListener('change', (event) => { $('install').disabled = !event.target.checked; });
$('install').addEventListener('click', install);
$('account').addEventListener('input', () => { $('detected-label').hidden = true; });
discover();
