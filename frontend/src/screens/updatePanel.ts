import {EventsOn} from '../../wailsjs/runtime/runtime';
import {
  GetUpdateStatus,
  GetAppVersion,
  ListAvailableVersions,
  InstallVersion,
  GetAutoUpdate,
  SetAutoUpdate,
  GetUpdateServerURL,
  SetUpdateServerURL,
  CheckForUpdatesNow,
} from '../../wailsjs/go/main/App';
import type {main, updater} from '../../wailsjs/go/models';
import {locale, onLanguageChange, t} from '../i18n';

function statusLabel(s: main.UpdateStatus): string {
  switch (s.state) {
    case 'downloading':
      return t('update.downloading');
    case 'outdated':
      return t(s.newerCount === 1 ? 'update.outdatedOne' : 'update.outdatedMany', {n: s.newerCount});
    case 'error':
      return t('update.error');
    default:
      return '';
  }
}

export function renderUpdateIndicator(container: HTMLElement, openBrowser: () => void) {
  let last: main.UpdateStatus | null = null;

  function apply(s: main.UpdateStatus) {
    last = s;
    const label = statusLabel(s);
    container.textContent = label;
    container.classList.toggle('update-indicator--hidden', !label);
    container.classList.toggle('update-indicator--warn', s.state === 'outdated' || s.state === 'error');
  }

  GetUpdateStatus().then(apply);
  EventsOn('update:status', (s: main.UpdateStatus) => apply(s));
  onLanguageChange(() => last && apply(last));

  container.addEventListener('click', () => {
    // Clicking is only meaningful once there's something to show/act on —
    // "downloading"/"none" don't need the version browser, but opening it
    // is harmless either way, so keep this simple rather than gating it.
    openBrowser();
  });
}

function configChangedBetween(versions: updater.VersionInfo[], fromIdx: number, toIdx: number): boolean {
  const [lo, hi] = fromIdx < toIdx ? [fromIdx, toIdx] : [toIdx, fromIdx];
  return versions.slice(lo, hi + 1).some((v) => v.configChanged);
}

export function renderVersionBrowser(overlay: HTMLElement) {
  overlay.innerHTML = `
    <div class="modal">
      <div class="modal-header">
        <h2 class="card-title">${t('versions.title')}</h2>
        <button class="titlebar-btn" id="btn-close-versions" aria-label="${t('common.close')}">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
        </button>
      </div>

      <div class="field">
        <label>
          <input type="checkbox" id="chk-autoupdate" />
          ${t('versions.autoUpdate')}
        </label>
      </div>
      <div class="field">
        <label for="update-server-url">${t('versions.server')}</label>
        <input id="update-server-url" type="text" />
      </div>

      <div id="version-list"><p class="muted">${t('versions.loading')}</p></div>
      <div id="changelog-view"></div>
    </div>
  `;

  const list = overlay.querySelector<HTMLDivElement>('#version-list')!;
  const changelogView = overlay.querySelector<HTMLDivElement>('#changelog-view')!;
  const chkAutoUpdate = overlay.querySelector<HTMLInputElement>('#chk-autoupdate')!;
  const serverUrlInput = overlay.querySelector<HTMLInputElement>('#update-server-url')!;

  // Empty rather than remove — #modal-overlay is shared by every popup.
  overlay.querySelector('#btn-close-versions')!.addEventListener('click', () => (overlay.innerHTML = ''));

  GetAutoUpdate().then((v) => (chkAutoUpdate.checked = v));
  chkAutoUpdate.addEventListener('change', () => {
    SetAutoUpdate(chkAutoUpdate.checked).then(() => CheckForUpdatesNow());
  });

  GetUpdateServerURL().then((v) => (serverUrlInput.value = v));
  serverUrlInput.addEventListener('change', () => {
    SetUpdateServerURL(serverUrlInput.value.trim()).then(() => CheckForUpdatesNow());
  });

  Promise.all([GetAppVersion(), ListAvailableVersions()])
    .then(([currentVersion, versions]) => {
      const currentIdx = versions.findIndex((v) => v.version === currentVersion);

      list.innerHTML = versions
        .map((v, i) => {
          const isCurrent = i === currentIdx;
          return `
            <div class="version-row ${isCurrent ? 'version-row--current' : ''}">
              <div class="file-card-main">
                <span class="file-card-name">${v.version}${isCurrent ? ` ${t('versions.current')}` : ''}</span>
                <span class="file-card-detail">${new Date(v.publishedAt).toLocaleDateString(locale())}</span>
              </div>
              <button class="submit-btn submit-btn--inline" data-changelog="${i}">${t('versions.changelog')}</button>
              ${isCurrent ? '' : `<button class="submit-btn submit-btn--inline" data-install="${i}">${i < currentIdx ? t('versions.downgrade') : t('versions.install')}</button>`}
            </div>
          `;
        })
        .join('');

      list.querySelectorAll<HTMLButtonElement>('[data-changelog]').forEach((btn) => {
        btn.addEventListener('click', () => {
          const v = versions[Number(btn.dataset.changelog)];
          changelogView.innerHTML = `
            <div class="card">
              <h3 class="card-title">${t('versions.changelogOf', {version: v.version})}</h3>
              <p class="muted" style="white-space: pre-wrap">${v.changelog || t('versions.noChangelog')}</p>
            </div>
          `;
        });
      });

      list.querySelectorAll<HTMLButtonElement>('[data-install]').forEach((btn) => {
        btn.addEventListener('click', () => {
          const targetIdx = Number(btn.dataset.install);
          const target = versions[targetIdx];
          const warn = currentIdx !== -1 && configChangedBetween(versions, currentIdx, targetIdx);
          if (warn) {
            const ok = confirm(t('versions.configChanged', {version: target.version}));
            if (!ok) return;
          }
          btn.disabled = true;
          btn.textContent = t('versions.installing');
          InstallVersion(target.version)
            .then(() => {
              btn.textContent = t('versions.ready');
            })
            .catch((err) => {
              btn.disabled = false;
              btn.textContent = t('versions.failed');
              console.error(err);
            });
        });
      });
    })
    .catch((err) => {
      list.innerHTML = `<p class="warning">${t('versions.loadFailed')}</p>`;
      console.error(err);
    });
}
