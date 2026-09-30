// Documents attached when a device is finished: files from the USB stick
// (erase certificate, test report, spec sheet), picked by the technician at
// submit, plus the generated spec-sheet TXT.
import {GetBetterPDFNames, ListUSBDrives, ScanUSBDrive} from '../../wailsjs/go/main/App';
import type {usbscan} from '../../wailsjs/go/models';
import {escapeHtml} from '../html';
import {choiceDialog} from './dialog';
import {t, type Key} from '../i18n';

// GLPI Document Categories on this instance: 1 Testbericht, 2 TXT Vorlage
// Online Shop, 3 Löschzertifikat.
const categoryByKind: Record<string, number> = {test_report: 1, spec_sheet: 2, erase_certificate: 3};

export interface Attachment {
  filename: string;
  sourcePath?: string;
  content?: number[];
  documentCategoryID: number;
}

const kindLabels: Record<string, Key> = {
  erase_certificate: 'att.kind.erase',
  test_report: 'att.kind.test',
  spec_sheet: 'att.kind.spec',
};

const invalidFilenameChars = /[\\/:*?"<>|]/g;

function clean(s: string): string {
  return s.replace(invalidFilenameChars, '').trim();
}

function basename(path: string): string {
  return path.split(/[\\/]/).pop() || path;
}

function extension(path: string): string {
  const b = basename(path);
  const i = b.lastIndexOf('.');
  return i > 0 ? b.slice(i) : '';
}

// Filenames as stored in GLPI. By default toolstar's own names, as on every
// document in GLPI so far. With "Bessere PDF-Namen" (settings): named after
// the device — "#3303 Löschzertifikat SamsungSSD850EVO250GB.pdf",
// "#3303 Testbericht.pdf", "#3303.txt". Duplicates get " 2", " 3", ….
export function documentNames(files: usbscan.FoundFile[], deviceName: string, better: boolean): string[] {
  const used = new Set<string>();
  return files.map((f) => {
    const ext = extension(f.path);
    let stem = basename(f.path).slice(0, basename(f.path).length - ext.length);
    if (better) {
      const name = clean(deviceName);
      if (f.kind === 'erase_certificate') {
        const disk = clean((f.report?.eraseResults?.[0]?.model ?? '').replace(/\s+/g, ''));
        stem = [name, 'Löschzertifikat', disk].filter(Boolean).join(' ');
      } else if (f.kind === 'test_report') {
        stem = `${name} Testbericht`;
      } else if (f.kind === 'spec_sheet') {
        stem = name;
      }
    }
    let candidate = stem + ext;
    for (let n = 2; used.has(candidate.toLowerCase()); n++) candidate = `${stem} ${n}${ext}`;
    used.add(candidate.toLowerCase());
    return candidate;
  });
}

export function hasSpecSheet(files: usbscan.FoundFile[]): boolean {
  return files.some((f) => f.kind === 'spec_sheet');
}

export async function toAttachments(files: usbscan.FoundFile[], deviceName: string): Promise<Attachment[]> {
  const names = documentNames(files, deviceName, await GetBetterPDFNames());
  return files.map((f, i) => ({
    filename: names[i],
    sourcePath: f.path,
    documentCategoryID: categoryByKind[f.kind] ?? 0,
  }));
}

export function specSheetAttachment(text: string, deviceName: string): Attachment {
  return {
    filename: `${clean(deviceName)}.txt`,
    content: Array.from(new TextEncoder().encode(text)),
    documentCategoryID: categoryByKind.spec_sheet,
  };
}

// A physical USB stick can show up as several removable partitions (a
// Ventoy stick, confirmed live 2026-09-29, has three: its boot partition,
// its main "Ventoy" data partition, and the technician's own second
// partition actually holding toolstar's testlx folder) — enumeration order
// isn't guaranteed to put the one with reports on it first, so every
// removable drive found gets scanned, not just the first.
export async function scanAllUSBDrives(): Promise<usbscan.FoundFile[]> {
  const drives = await ListUSBDrives();
  if (!drives || drives.length === 0) return [];
  const perDrive = await Promise.all(drives.map((d) => ScanUSBDrive(d.path).catch(() => [])));
  return perDrive.flat();
}

// testlx accumulates reports from every device a technician has ever
// processed on that stick (confirmed live 2026-09-29: 10 erase
// certificates and 42 test reports from many different machines on one
// real stick), so scanAllUSBDrives() alone returns far more than belongs
// to the device currently being worked on. Matched by either the PC's own
// serial (PC-Seriennr.) or, since that's sometimes blank/generic on
// white-box desktops, any currently-installed disk's serial against an
// erase certificate's per-drive serial (only erase certificates carry
// one — a test report's Disks aren't parsed at all, see
// toolstar.Report.Disks). spec_sheet/unknown carry no serial to compare,
// so they pass through unfiltered (spec_sheet is matched by device name
// elsewhere; unknown is never shown in the first place).
export function filterForDevice(files: usbscan.FoundFile[], pcSerial: string, diskSerials: string[] = []): usbscan.FoundFile[] {
  const wantPC = pcSerial.trim().toLowerCase();
  const wantDisks = new Set(diskSerials.filter(Boolean).map((s) => s.trim().toLowerCase()));
  if (!wantPC && wantDisks.size === 0) return files;

  return files.filter((f) => {
    if (f.kind !== 'erase_certificate' && f.kind !== 'test_report') return true;
    const gotPC = (f.report?.serialNumber ?? '').trim().toLowerCase();
    if (wantPC && gotPC === wantPC) return true;
    return !!f.report?.eraseResults?.some((e) => wantDisks.has((e.serial ?? '').trim().toLowerCase()));
  });
}

function detail(f: usbscan.FoundFile): string {
  if (f.kind === 'erase_certificate' && f.report?.eraseResults?.length) {
    return f.report.eraseResults.map((e) => `${e.model} · ${e.serial}`).join(', ');
  }
  if (f.kind === 'test_report' && f.report) {
    return [f.report.pcName, f.report.serialNumber].filter(Boolean).join(' · ');
  }
  return '';
}

// Scans the USB stick at submit and lets the technician pick which files to
// attach. Resolves to the chosen files, [] to continue without any, or null
// to go back. pcSerial/diskSerials narrow down to this device's own
// reports — see filterForDevice; pass '' / [] to show everything (e.g.
// nothing detected).
export async function chooseUSBFiles(deviceName: string, pcSerial: string, diskSerials: string[] = []): Promise<usbscan.FoundFile[] | null> {
  const better = await GetBetterPDFNames().catch(() => false);
  for (;;) {
    let files: usbscan.FoundFile[] = [];
    let problem = '';
    try {
      const drives = await ListUSBDrives();
      if (!drives || drives.length === 0) {
        problem = t('att.noStick');
      } else {
        files = filterForDevice(await scanAllUSBDrives(), pcSerial, diskSerials).filter((f) => f.kind in kindLabels);
        if (files.length === 0) problem = t('att.noFiles', {path: drives.map((d) => d.path).join(', ')});
      }
    } catch (err) {
      console.error(err);
      problem = t('att.searchFailed');
    }

    if (problem) {
      const key = await choiceDialog({
        title: t('att.none.title'),
        bodyHtml: `
          <p class="warning">${escapeHtml(problem)}</p>
          <p class="muted">${t('att.none.note')}</p>`,
        choices: [
          {key: 'back', label: t('common.back')},
          {key: 'retry', label: t('att.retry')},
          {key: 'without', label: t('att.without'), primary: true},
        ],
      });
      if (key === 'retry') continue;
      return key === 'without' ? [] : null;
    }

    const names = documentNames(files, deviceName, better);
    const rows = files
      .map((f, i) => {
        const d = detail(f);
        return `
        <label class="file-card file-choice">
          <input type="checkbox" class="switch" data-file="${i}" checked />
          <span class="file-card-main">
            <span class="file-card-kind">${t(kindLabels[f.kind])}</span>
            <span class="file-card-name">${escapeHtml(names[i])}</span>
            ${d ? `<span class="file-card-detail">${escapeHtml(d)}</span>` : ''}
          </span>
        </label>`;
      })
      .join('');

    let chosen: usbscan.FoundFile[] = [];
    const key = await choiceDialog({
      title: t('att.pick.title'),
      bodyHtml: `<p class="muted">${t('att.pick.intro')}</p><div>${rows}</div>`,
      choices: [
        {key: 'back', label: t('common.back')},
        {key: 'without', label: t('att.without')},
        {key: 'attach', label: t('att.attach'), primary: true},
      ],
      beforeClose: (k, dialog) => {
        if (k !== 'attach') return;
        chosen = files.filter((_, i) => dialog.querySelector<HTMLInputElement>(`[data-file="${i}"]`)?.checked);
      },
    });
    if (key === 'attach') return chosen;
    return key === 'without' ? [] : null;
  }
}

// Finds this device's own toolstar test report on the stick without asking
// anything — used to prefill the spec sheet of a device that's already in
// GLPI. Matched by serial (see filterForDevice) — without it, this would
// happily return some other device's report, from testlx's own history.
export async function findTestReport(pcSerial: string, diskSerials: string[] = []): Promise<usbscan.FoundFile['report'] | undefined> {
  try {
    const files = filterForDevice(await scanAllUSBDrives(), pcSerial, diskSerials);
    return files.find((f) => f.kind === 'test_report' && f.report)?.report;
  } catch {
    return undefined;
  }
}
