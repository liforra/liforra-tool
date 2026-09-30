// Menü → Über: app identity plus a ready-to-use download link (this
// install's token already included — see about.go / GetAboutInfo).
import {GetAboutInfo} from '../../wailsjs/go/main/App';
import {BrowserOpenURL, ClipboardSetText} from '../../wailsjs/runtime/runtime';
import {escapeHtml} from '../html';
import {t} from '../i18n';

export function renderAbout(content: HTMLElement) {
  content.innerHTML = `
    <div class="intake-screen">
      <div class="intake-header">
        <div>
          <h1 class="screen-title" id="about-title"></h1>
          <p class="screen-subtitle" id="about-version"></p>
        </div>
      </div>

      <section class="card" id="about-download-card" hidden>
        <h2 class="card-title">${escapeHtml(t('about.downloadTitle'))}</h2>
        <p class="muted spec-note">${escapeHtml(t('about.downloadNote'))}</p>
        <div class="field">
          <input id="about-download-link" type="text" readonly />
        </div>
        <div class="modal-actions">
          <button class="submit-btn submit-btn--inline submit-btn--secondary" id="btn-about-copy">${escapeHtml(t('about.copyLink'))}</button>
          <button class="submit-btn submit-btn--inline" id="btn-about-open">${escapeHtml(t('about.openLink'))}</button>
        </div>
      </section>

      <section class="card" id="about-no-link" hidden>
        <p class="muted">${escapeHtml(t('about.noLink'))}</p>
      </section>

      <section class="card">
        <h2 class="card-title">${escapeHtml(t('about.sourceTitle'))}</h2>
        <p class="muted spec-note" id="about-license"></p>
        <button class="submit-btn submit-btn--inline submit-btn--secondary" id="btn-about-source">${escapeHtml(t('about.sourceLink'))}</button>
      </section>
    </div>
  `;

  GetAboutInfo().then((info) => {
    content.querySelector('#about-title')!.textContent = t('about.title', {app: info.appName});
    content.querySelector('#about-version')!.textContent =
      info.appVersion === 'dev' ? t('about.versionDev') : t('about.version', {version: info.appVersion});

    if (info.downloadUrl) {
      const card = content.querySelector<HTMLElement>('#about-download-card')!;
      card.hidden = false;
      const linkInput = content.querySelector<HTMLInputElement>('#about-download-link')!;
      linkInput.value = info.downloadUrl;

      content.querySelector('#btn-about-copy')!.addEventListener('click', () => {
        ClipboardSetText(info.downloadUrl).then(() => {
          const btn = content.querySelector<HTMLButtonElement>('#btn-about-copy')!;
          const original = btn.textContent;
          btn.textContent = t('about.copied');
          setTimeout(() => (btn.textContent = original), 1500);
        });
      });
      content.querySelector('#btn-about-open')!.addEventListener('click', () => BrowserOpenURL(info.downloadUrl));
    } else {
      content.querySelector<HTMLElement>('#about-no-link')!.hidden = false;
    }

    content.querySelector('#about-license')!.textContent = t('about.licenseNote', {license: info.license});
    content.querySelector('#btn-about-source')!.addEventListener('click', () => BrowserOpenURL(info.sourceCodeUrl));
  });
}
