// The hardware scan of this PC. It needs no GLPI login, so it starts as soon
// as the app opens; "Neues Gerät" picks up the result instead of scanning
// again.
import {DetectHardware} from '../wailsjs/go/main/App';
import type {hwinfo} from '../wailsjs/go/models';

let current: Promise<hwinfo.Info> | null = null;

export function startScan(): Promise<hwinfo.Info> {
  current = DetectHardware();
  return current;
}

export function latestScan(): Promise<hwinfo.Info> {
  return current ?? startScan();
}
