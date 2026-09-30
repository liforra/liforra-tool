// The language tab: pick the interface language. Switching is handled by
// main.ts, which rebuilds every screen in the new language.
import {getLang, languages, setLang, t} from '../i18n';

export function renderLanguage(content: HTMLElement) {
  content.innerHTML = `
    <div class="intake-screen">
      <div class="intake-header">
        <div>
          <h1 class="screen-title">${t('lang.title')}</h1>
          <p class="screen-subtitle">${t('lang.subtitle')}</p>
        </div>
      </div>

      <section class="card">
        <div class="lang-list" role="radiogroup" aria-label="${t('lang.title')}">
          ${languages
            .map(
              (l) => `
          <button class="lang-option${l.id === getLang() ? ' lang-option--active' : ''}" role="radio" aria-checked="${l.id === getLang()}" data-lang="${l.id}">
            <span class="lang-option-name">${l.label}</span>
            <span class="lang-option-code">${l.id.toUpperCase()}</span>
          </button>`
            )
            .join('')}
        </div>
        <p class="muted spec-note">${t('lang.note')}</p>
      </section>
    </div>
  `;

  content.querySelectorAll<HTMLButtonElement>('[data-lang]').forEach((btn) => {
    btn.addEventListener('click', () => setLang(btn.dataset.lang as 'de' | 'en'));
  });
}
