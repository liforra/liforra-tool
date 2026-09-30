// Plain script, no bundler — install.exe embeds this directory verbatim and
// must run with zero network access, so no import maps, no CDN, no build
// step. Wails injects window.go.main.App.* and window.runtime.* itself.

document.querySelector('#btn-minimise').addEventListener('click', () => window.runtime.WindowMinimise());
document.querySelector('#btn-close').addEventListener('click', () => window.runtime.Quit());

const targetListEl = document.querySelector('#target-list');
const manualInput = document.querySelector('#manual-path-input');
const portableCheckbox = document.querySelector('#portable-checkbox');
const installBtn = document.querySelector('#install-btn');
const statusEl = document.querySelector('#status');

let targets = [];
let selectedPath = null;

function renderTargets() {
  targetListEl.innerHTML = targets
    .map((t) => {
      const selected = t.path === selectedPath ? ' selected' : '';
      const badge = t.hasPortableApps ? '<span class="badge">PortableApps gefunden</span>' : '';
      return `<button type="button" class="target-card${selected}" data-path="${encodeURIComponent(t.path)}">
        <span>
          <span class="target-card-label">${escapeHtml(t.label)}</span><br />
          <span class="target-card-path">${escapeHtml(t.path)}</span>
        </span>
        ${badge}
      </button>`;
    })
    .join('');

  targetListEl.querySelectorAll('.target-card').forEach((el) => {
    el.addEventListener('click', () => {
      selectedPath = decodeURIComponent(el.getAttribute('data-path'));
      manualInput.value = '';
      renderTargets();
    });
  });
}

function escapeHtml(s) {
  return s.replace(/[&<>"']/g, (c) => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
}

manualInput.addEventListener('input', () => {
  if (manualInput.value.trim() !== '') {
    selectedPath = null;
    renderTargets();
  }
});

function showStatus(kind, message) {
  statusEl.className = `status status--visible status--${kind}`;
  statusEl.textContent = message;
}

let installed = false;

installBtn.addEventListener('click', async () => {
  if (installed) {
    window.runtime.Quit();
    return;
  }

  const manual = manualInput.value.trim();
  const targetDir = manual !== '' ? manual : selectedPath;
  if (!targetDir) {
    showStatus('error', 'Bitte einen Zielort auswählen oder einen Pfad eingeben.');
    return;
  }

  installBtn.disabled = true;
  installBtn.textContent = 'Installiere…';
  showStatus('ok', 'Installation läuft…');

  try {
    const result = await window.go.main.App.DoInstall(targetDir, portableCheckbox.checked);
    showStatus('ok', `Installiert nach: ${result.installDir}`);
    installed = true;
    installBtn.disabled = false;
    installBtn.textContent = 'Fertig';
  } catch (err) {
    showStatus('error', String(err));
    installBtn.disabled = false;
    installBtn.textContent = 'Installieren';
  }
});

window.go.main.App.ListTargets()
  .then((list) => {
    targets = list || [];
    const preselect = targets.find((t) => t.hasPortableApps) || targets[0];
    selectedPath = preselect ? preselect.path : null;
    renderTargets();
  })
  .catch((err) => showStatus('error', String(err)));
