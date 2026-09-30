import {Login} from '../../wailsjs/go/main/App';
import {t} from '../i18n';

const eyeIcon = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/></svg>';
const eyeOffIcon = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49"/><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242"/><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143"/><path d="m2 2 20 20"/></svg>';

export function renderLoginScreen(content: HTMLElement, onLoggedIn: (asUser: string) => void) {
  content.innerHTML = `
    <div class="login-screen">
      <div class="brand">
        <span class="brand-tag">liforra-tool</span>
        <h1 class="brand-name">${t('login.title')}</h1>
        <p class="brand-tagline">${t('login.tagline')}</p>
      </div>

      <form class="login-form" id="login-form">
        <div class="field">
          <label for="username">${t('login.username')}</label>
          <input id="username" name="username" type="text" autocomplete="username" required autofocus />
        </div>
        <div class="field">
          <label for="password">${t('login.password')}</label>
          <div class="password-wrap">
            <input id="password" name="password" type="password" autocomplete="current-password" required />
            <button type="button" class="password-toggle" id="btn-toggle-password" aria-label="${t('login.showPassword')}" aria-pressed="false">
              ${eyeIcon}
            </button>
          </div>
        </div>
        <button class="submit-btn" type="submit">${t('login.submit')}</button>
        <p class="error-message" id="error-message"></p>
      </form>
    </div>
  `;

  const form = content.querySelector<HTMLFormElement>('#login-form')!;
  const usernameInput = content.querySelector<HTMLInputElement>('#username')!;
  const passwordInput = content.querySelector<HTMLInputElement>('#password')!;
  const submitBtn = content.querySelector<HTMLButtonElement>('.submit-btn')!;
  const errorMessage = content.querySelector<HTMLParagraphElement>('#error-message')!;
  const togglePasswordBtn = content.querySelector<HTMLButtonElement>('#btn-toggle-password')!;

  togglePasswordBtn.addEventListener('click', () => {
    const isHidden = passwordInput.type === 'password';
    passwordInput.type = isHidden ? 'text' : 'password';
    togglePasswordBtn.innerHTML = isHidden ? eyeOffIcon : eyeIcon;
    togglePasswordBtn.setAttribute('aria-pressed', String(isHidden));
    togglePasswordBtn.setAttribute('aria-label', isHidden ? t('login.hidePassword') : t('login.showPassword'));
    passwordInput.focus();
  });

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    errorMessage.textContent = '';
    submitBtn.disabled = true;
    submitBtn.textContent = t('login.busy');

    Login(usernameInput.value, passwordInput.value)
      .then((result) => {
        if (result.success) {
          passwordInput.value = '';
          onLoggedIn(result.asUser!);
          return;
        }
        errorMessage.textContent = result.error || t('login.failed');
        submitBtn.disabled = false;
        submitBtn.textContent = t('login.submit');
      })
      .catch((err) => {
        errorMessage.textContent = t('login.connection');
        console.error(err);
        submitBtn.disabled = false;
        submitBtn.textContent = t('login.submit');
      });
  });
}
