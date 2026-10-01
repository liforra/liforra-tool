import {
  GetAdministrator,
  GetBetterPDFNames,
  SetAdministrator,
  SetBetterPDFNames,
  GetAutoUpdate,
  SetAutoUpdate,
  GetUpdateServerURL,
  SetUpdateServerURL,
  CheckForUpdatesNow,
  GetActiveProfile,
} from '../../wailsjs/go/main/App';
import {t, type Key} from '../i18n';

const toggles: {id: string; title: Key; description: Key; get: () => Promise<boolean>; set: (v: boolean) => Promise<void>}[] = [
  {
    id: 'chk-admin',
    title: 'settings.admin.title',
    description: 'settings.admin.desc',
    get: GetAdministrator,
    set: SetAdministrator,
  },
  {
    id: 'chk-pdf-names',
    title: 'settings.pdf.title',
    description: 'settings.pdf.desc',
    get: GetBetterPDFNames,
    set: SetBetterPDFNames,
  },
  {
    id: 'chk-auto-update',
    title: 'settings.autoUpdate.title',
    description: 'settings.autoUpdate.desc',
    get: GetAutoUpdate,
    set: SetAutoUpdate,
  },
];

export function renderSettings(content: HTMLElement, onBack: () => void) {
  content.innerHTML = `
    <div class="intake-screen">
      <div class="intake-header">
        <h1 class="screen-title">${t('settings.title')}</h1>
        <button class="submit-btn submit-btn--inline submit-btn--secondary" id="btn-settings-back">${t('common.back')}</button>
      </div>

      <section class="card">
        <h3 class="card-title">${t('settings.glpiAccount.title')}</h3>
        <p class="muted" id="settings-active-profile">${t('settings.glpiAccount.loading')}</p>
      </section>

      <section class="card stack">
        ${toggles
          .map(
            (s) => `
        <label class="toggle-row" for="${s.id}">
          <span class="toggle-text">
            <span class="toggle-title">${t(s.title)}</span>
            <span class="muted">${t(s.description)}</span>
          </span>
          <input type="checkbox" class="switch" id="${s.id}" disabled />
        </label>`
          )
          .join('')}

        <div class="field">
          <label for="settings-update-server">${t('settings.updateServer.title')}</label>
          <input type="text" id="settings-update-server" disabled />
          <span class="muted">${t('settings.updateServer.desc')}</span>
        </div>

        <button class="submit-btn submit-btn--secondary" id="btn-check-updates">${t('settings.checkNow')}</button>

        <p class="error-message" id="settings-error"></p>
      </section>
    </div>
  `;

  const settingsError = content.querySelector<HTMLParagraphElement>('#settings-error')!;
  content.querySelector('#btn-settings-back')!.addEventListener('click', onBack);

  const activeProfileEl = content.querySelector<HTMLParagraphElement>('#settings-active-profile')!;
  GetActiveProfile()
    .then((name) => (activeProfileEl.textContent = t('settings.glpiAccount.profile', {profile: name})))
    .catch((err) => {
      activeProfileEl.textContent = t('settings.glpiAccount.unavailable');
      console.error(err);
    });

  const serverUrlInput = content.querySelector<HTMLInputElement>('#settings-update-server')!;
  GetUpdateServerURL()
    .then((url) => {
      serverUrlInput.value = url;
      serverUrlInput.disabled = false;
    })
    .catch((err) => {
      settingsError.textContent = t('settings.loadError');
      console.error(err);
    });
  serverUrlInput.addEventListener('change', () => {
    settingsError.textContent = '';
    SetUpdateServerURL(serverUrlInput.value.trim()).catch((err) => {
      settingsError.textContent = t('settings.saveError');
      console.error(err);
    });
  });

  const checkNowBtn = content.querySelector<HTMLButtonElement>('#btn-check-updates')!;
  checkNowBtn.addEventListener('click', () => {
    checkNowBtn.disabled = true;
    const originalLabel = checkNowBtn.textContent;
    checkNowBtn.textContent = t('settings.checkNow.checking');
    CheckForUpdatesNow().finally(() => {
      checkNowBtn.disabled = false;
      checkNowBtn.textContent = originalLabel;
    });
  });

  for (const s of toggles) {
    const chk = content.querySelector<HTMLInputElement>(`#${s.id}`)!;
    s.get()
      .then((enabled) => {
        chk.checked = enabled;
        chk.disabled = false;
      })
      .catch((err) => {
        settingsError.textContent = t('settings.loadError');
        console.error(err);
      });
    chk.addEventListener('change', () => {
      settingsError.textContent = '';
      s.set(chk.checked).catch((err) => {
        chk.checked = !chk.checked;
        settingsError.textContent = t('settings.saveError');
        console.error(err);
      });
    });
  }
}
