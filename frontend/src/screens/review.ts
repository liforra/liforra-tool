// Finishing a device that already exists in GLPI (found via "Suchen"):
// Zusatzdaten, documents from the USB stick and the spec-sheet TXT, then
// status "Einsetzbar". New devices do all of this on "Neues Gerät" instead.
import {FinishDevice, GenerateSpecSheetTXT, LookupLenovoSpecs} from '../../wailsjs/go/main/App';
import {main} from '../../wailsjs/go/models';
import type {DeviceIntakeResult} from './deviceIntake';
import {isLenovo, renderSpecSheetForm, specDefaults} from './specSheet';
import {askMissingTxtInfo} from './dialog';
import {chooseUSBFiles, findTestReport, hasSpecSheet, specSheetAttachment, toAttachments} from './attachments';
import {escapeHtml} from '../html';
import {t} from '../i18n';
import {latestScan} from '../scan';

export function renderReview(content: HTMLElement, intake: DeviceIntakeResult, onDone: () => void) {
  const {computer} = intake;

  content.innerHTML = `
    <div class="intake-screen">
      <div class="intake-header">
        <div>
          <h1 class="screen-title">${escapeHtml(t('review.title', {name: computer.name}))}</h1>
          <p class="screen-subtitle">${escapeHtml(computer.manufacturer?.name ?? '')} ${escapeHtml(computer.model?.name ?? '')}</p>
        </div>
      </div>

      <section class="card">
        <h2 class="card-title">${t('nd.extraTitle')}</h2>
        <div class="field">
          <label for="f-maengel">${t('nd.defects')}</label>
          <textarea id="f-maengel" rows="3"></textarea>
        </div>
        <div class="grid-2">
          <div class="field">
            <label for="f-akku1">${t('nd.battery')}</label>
            <input id="f-akku1" type="number" min="0" max="100" />
          </div>
          <div class="field">
            <label for="f-akku2">${t('nd.battery2')}</label>
            <input id="f-akku2" type="number" min="0" max="100" />
          </div>
        </div>
        <div class="field">
          <label for="f-osaktiv">${t('nd.osActivated')}</label>
          <select id="f-osaktiv">
            <option value="0">${t('common.no')}</option>
            <option value="1">${t('common.yes')}</option>
          </select>
        </div>
      </section>

      <section class="card" id="spec-card">
        <div class="intake-header">
          <h2 class="card-title">${t('review.txtTitle')}</h2>
          <button class="submit-btn submit-btn--inline submit-btn--secondary" id="btn-txt-preview">${t('nd.preview')}</button>
        </div>
        <p class="muted spec-note">${escapeHtml(t('review.txtNote', {name: computer.name}))}</p>
        <div id="spec-form" class="spec-form"></div>
        <pre id="txt-preview" class="txt-preview"></pre>
      </section>

      <button class="submit-btn" id="btn-finish">${escapeHtml(t('review.finish'))}</button>
      <p class="error-message" id="review-error"></p>
    </div>
  `;

  const txtPreview = content.querySelector<HTMLPreElement>('#txt-preview')!;
  const specForm = renderSpecSheetForm(content.querySelector<HTMLDivElement>('#spec-form')!, specDefaults({computer}));

  // This app runs on the device being finished, so its own hardware scan
  // is this device's — used to match a report on the stick that
  // identifies itself by drive serial rather than PC serial (some
  // white-box desktops have no meaningful chassis serial), and kept
  // around (not just the serials) for specDefaults' scan.info — ready for
  // a drive-serial TXT field once its exact format is decided.
  const scanInfo = latestScan().catch(() => null);
  const diskSerials = scanInfo.then((info) => (info?.disks ?? []).map((d) => d.serial));

  // A toolstar test report on the stick knows CPU, RAM and GPU — fill those
  // into the spec sheet quietly, without touching what's been typed.
  diskSerials.then((serials) => findTestReport(computer.serial, serials)).then((testReport) => {
    if (testReport) specForm.fillEmpty(specDefaults({computer, testReport}));
  });

  scanInfo.then((info) => {
    if (info) specForm.fillEmpty(specDefaults({computer, scan: {info}}));
  });

  // Lenovo's own published spec sheet knows weight/dimensions/ports GLPI
  // doesn't have on file — see LookupLenovoSpecs. Best-effort, same as above.
  const brand = computer.manufacturer?.name ?? '';
  const modell = specDefaults({computer}).modell;
  if (isLenovo(brand) && modell) {
    LookupLenovoSpecs(modell)
      .then((fields) => specForm.fillEmpty(fields))
      .catch((err) => console.error(err));
  }

  content.querySelector('#btn-txt-preview')!.addEventListener('click', () => {
    GenerateSpecSheetTXT(specForm.read())
      .then((text) => (txtPreview.textContent = text))
      .catch((err) => console.error(err));
  });

  const btnFinish = content.querySelector<HTMLButtonElement>('#btn-finish')!;
  const errorMessage = content.querySelector<HTMLParagraphElement>('#review-error')!;

  function setBusy(busy: boolean) {
    btnFinish.disabled = busy;
    btnFinish.textContent = busy ? t('review.sending') : t('review.finish');
  }

  btnFinish.addEventListener('click', async () => {
    errorMessage.textContent = '';

    setBusy(true);
    btnFinish.textContent = t('review.searchingUsb');
    const files = await chooseUSBFiles(computer.name, computer.serial, await diskSerials);
    setBusy(false);
    if (files === null) return;

    // A spec sheet from the stick replaces the generated one.
    let generateTxt = !hasSpecSheet(files);
    const missing = generateTxt ? specForm.missing() : [];
    if (missing.length > 0) {
      const choice = await askMissingTxtInfo(missing.map((m) => m.label));
      if (choice === 'back') {
        missing[0].el?.scrollIntoView({block: 'center'});
        missing[0].el?.focus();
        return;
      }
      generateTxt = choice === 'anyway';
    }

    setBusy(true);
    try {
      const documents = await toAttachments(files, computer.name);
      if (generateTxt) documents.push(specSheetAttachment(await GenerateSpecSheetTXT(specForm.read()), computer.name));

      const val = (id: string) => content.querySelector<HTMLInputElement>(id)!.value.trim();
      await FinishDevice(
        main.FinishDeviceInput.createFrom({
          computerID: computer.id,
          zusatzdaten: {
            mngelfield: val('#f-maengel') || undefined,
            // A string, not a number — see newDevice.ts's akku() for why.
            akkugesundheitinfield: val('#f-akku1') || undefined,
            akkugesundheittwoinfield: val('#f-akku2') || undefined,
            betriebssystemaktiviertfield: val('#f-osaktiv') === '1',
          },
          documents: documents.map((a) => ({
            filename: a.filename,
            sourcePath: a.sourcePath ?? '',
            content: a.content,
            documentCategoryID: a.documentCategoryID,
          })),
        })
      );
      onDone();
    } catch (err) {
      errorMessage.textContent = t('review.error');
      console.error(err);
      setBusy(false);
    }
  });
}
