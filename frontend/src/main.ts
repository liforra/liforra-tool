import './style.css';
import {Quit, WindowMinimise, WindowToggleMaximise} from '../wailsjs/runtime/runtime';
import {TryResumeSession, GetAppName} from '../wailsjs/go/main/App';
import {renderLoginScreen} from './screens/login';
import {renderDeviceIntake} from './screens/deviceIntake';
import {renderReview} from './screens/review';
import {renderNewDevice} from './screens/newDevice';
import {renderSettings} from './screens/settings';
import {renderUpdateIndicator, renderVersionBrowser} from './screens/updatePanel';
import {renderLanguage} from './screens/language';
import {renderAbout} from './screens/about';
import {renderDeviceCheck} from './screens/deviceCheck';
import {startScan} from './scan';
import {onLanguageChange, t, type Key} from './i18n';

const app = document.querySelector<HTMLDivElement>('#app')!;

const searchIcon =
  '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/></svg>';
const pcIcon =
  '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="20" x="5" y="2" rx="2"/><path d="M15 14h.01"/><path d="M9 6h6"/><path d="M9 10h6"/></svg>';

// lucide "languages"
const languagesIcon =
  '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 8 6 6"/><path d="m4 14 6-6 2-3"/><path d="M2 5h12"/><path d="M7 2h1"/><path d="m22 22-5-10-5 10"/><path d="M14 18h6"/></svg>';
// lucide "shield-check"
const checkIcon =
  '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/></svg>';

type TabID = 'search' | 'pc' | 'check' | 'language';
// end: pushed to the right end of the tab bar.
const TABS: {id: TabID; label: Key; icon: string; end?: boolean}[] = [
  {id: 'pc', label: 'tab.newDevice', icon: pcIcon},
  {id: 'search', label: 'tab.search', icon: searchIcon},
  {id: 'check', label: 'tab.check', icon: checkIcon},
  {id: 'language', label: 'tab.language', icon: languagesIcon, end: true},
];

app.innerHTML = `
  <div class="titlebar" style="--wails-draggable:drag">
    <div class="titlebar-left">
      <span class="titlebar-tag" id="titlebar-app-name">liforra-tool</span>
      <span class="update-indicator update-indicator--hidden" id="update-indicator" style="--wails-draggable:no-drag"></span>
    </div>
    <div class="titlebar-controls" style="--wails-draggable:no-drag">
      <button class="titlebar-btn" id="btn-menu" aria-haspopup="menu" aria-expanded="false" aria-controls="titlebar-menu">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 6h16"/><path d="M4 12h16"/><path d="M4 18h16"/></svg>
      </button>
      <div class="titlebar-menu" id="titlebar-menu" role="menu" hidden>
        <button class="titlebar-menu-item" id="menu-settings" role="menuitem"></button>
        <button class="titlebar-menu-item" id="menu-about" role="menuitem"></button>
      </div>
      <button class="titlebar-btn" id="btn-minimise">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/></svg>
      </button>
      <button class="titlebar-btn" id="btn-maximise">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M21 8V5a2 2 0 0 0-2-2h-3"/><path d="M3 16v3a2 2 0 0 0 2 2h3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/></svg>
      </button>
      <button class="titlebar-btn titlebar-btn--close" id="btn-close">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
  </div>
  <div class="content" id="content"></div>
  <nav class="tabbar" id="tabbar">
    ${TABS.map((tab) => `<button class="tabbar-btn${tab.end ? ' tabbar-btn--end' : ''}" id="tab-${tab.id}" data-tab="${tab.id}" disabled>${tab.icon}</button>`).join('')}
  </nav>
  <div id="modal-overlay"></div>
`;

// Text that lives in the static markup above; runs again on a language change.
function applyChromeText() {
  const label = (sel: string, key: Key) => document.querySelector(sel)!.setAttribute('aria-label', t(key));
  label('#btn-menu', 'window.menu');
  label('#btn-minimise', 'window.minimise');
  label('#btn-maximise', 'window.maximise');
  label('#btn-close', 'common.close');
  document.querySelector('#menu-settings')!.textContent = t('menu.settings');
  document.querySelector('#menu-about')!.textContent = t('menu.about');
  for (const tab of TABS) {
    const btn = document.querySelector(`#tab-${tab.id}`)!;
    btn.setAttribute('aria-label', t(tab.label));
    btn.setAttribute('title', t(tab.label));
  }
}
applyChromeText();

document.querySelector('#btn-minimise')!.addEventListener('click', () => WindowMinimise());
document.querySelector('#btn-maximise')!.addEventListener('click', () => WindowToggleMaximise());
document.querySelector('#btn-close')!.addEventListener('click', () => Quit());

GetAppName().then((name) => {
  document.querySelectorAll('#titlebar-app-name').forEach((el) => (el.textContent = name));
});

renderUpdateIndicator(document.querySelector('#update-indicator')!, () => {
  const overlay = document.querySelector<HTMLDivElement>('#modal-overlay')!;
  renderVersionBrowser(overlay);
});

const content = document.querySelector<HTMLDivElement>('#content')!;
const tabbar = document.querySelector<HTMLElement>('#tabbar')!;
let activeTab: TabID = 'pc';
let currentUser = '';
// The screen on top right now, so a language change can come back to it.
let shown: ViewID = 'login';

// Each view keeps its own element, so switching tabs hides a screen instead
// of rebuilding it — a scan and everything typed survive the switch.
type ViewID = TabID | 'login' | 'settings' | 'about';
const views = new Map<ViewID, HTMLDivElement>();

function view(id: ViewID): HTMLDivElement {
  let el = views.get(id);
  if (!el) {
    el = document.createElement('div');
    el.className = 'view';
    content.appendChild(el);
    views.set(id, el);
  }
  return el;
}

function showView(id: ViewID) {
  views.forEach((el, key) => (el.hidden = key !== id));
}

function renderSearchTab(el: HTMLElement) {
  renderDeviceIntake(el, currentUser, (result) => renderReview(el, result, () => renderSearchTab(el)));
}

function setTab(tab: TabID) {
  activeTab = tab;
  shown = tab;
  tabbar.querySelectorAll<HTMLButtonElement>('.tabbar-btn').forEach((btn) => {
    btn.classList.toggle('tabbar-btn--active', btn.dataset.tab === tab);
  });
  const el = view(tab);
  if (!el.dataset.ready) {
    el.dataset.ready = '1';
    if (tab === 'search') renderSearchTab(el);
    else if (tab === 'check') renderDeviceCheck(el);
    else if (tab === 'language') renderLanguage(el);
    else renderNewDevice(el, currentUser);
  }
  showView(tab);
}

tabbar.querySelectorAll<HTMLButtonElement>('.tabbar-btn').forEach((btn) => {
  btn.addEventListener('click', () => {
    if (btn.disabled) return;
    setTab(btn.dataset.tab as TabID);
  });
});

function enableTabs() {
  tabbar.querySelectorAll<HTMLButtonElement>('.tabbar-btn').forEach((btn) => (btn.disabled = false));
  tabbar.querySelector(`#tab-${activeTab}`)?.classList.add('tabbar-btn--active');
}

function showIntake(asUser: string) {
  currentUser = asUser;
  enableTabs();
  setTab(activeTab);
}

function showLogin() {
  shown = 'login';
  renderLoginScreen(view('login'), showIntake);
  showView('login');
}

function showSettings() {
  shown = 'settings';
  tabbar.querySelectorAll('.tabbar-btn').forEach((btn) => btn.classList.remove('tabbar-btn--active'));
  renderSettings(view('settings'), () => (currentUser ? setTab(activeTab) : showLogin()));
  showView('settings');
}

function showAbout() {
  shown = 'about';
  tabbar.querySelectorAll('.tabbar-btn').forEach((btn) => btn.classList.remove('tabbar-btn--active'));
  renderAbout(view('about'));
  showView('about');
}

// Every screen is built with the old language's text, so throw them away and
// rebuild the one on top. The scan lives in scan.ts and survives.
onLanguageChange(() => {
  applyChromeText();
  views.forEach((el) => el.remove());
  views.clear();
  if (shown === 'settings') showSettings();
  else if (shown === 'about') showAbout();
  else if (currentUser) setTab(shown === 'login' ? activeTab : (shown as TabID));
  else showLogin();
});

const btnMenu = document.querySelector<HTMLButtonElement>('#btn-menu')!;
const titlebarMenu = document.querySelector<HTMLDivElement>('#titlebar-menu')!;

function setMenuOpen(open: boolean) {
  titlebarMenu.hidden = !open;
  btnMenu.setAttribute('aria-expanded', String(open));
}

btnMenu.addEventListener('click', (e) => {
  e.stopPropagation();
  setMenuOpen(titlebarMenu.hidden);
});
document.addEventListener('click', (e) => {
  if (!titlebarMenu.hidden && !titlebarMenu.contains(e.target as Node)) setMenuOpen(false);
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && !titlebarMenu.hidden) {
    setMenuOpen(false);
    btnMenu.focus();
  }
});
document.querySelector('#menu-settings')!.addEventListener('click', () => {
  setMenuOpen(false);
  showSettings();
});
document.querySelector('#menu-about')!.addEventListener('click', () => {
  setMenuOpen(false);
  showAbout();
});

// The hardware scan needs no GLPI session — start it now so "Neues Gerät"
// is ready by the time anyone gets there.
startScan().catch(() => {});

view('login').innerHTML = `<div class="login-screen"><p class="muted">${t('login.checking')}</p></div>`;
showView('login');
TryResumeSession()
  .then((result) => {
    if (result.success && result.asUser) {
      showIntake(result.asUser);
    } else {
      showLogin();
    }
  })
  .catch(() => showLogin());
