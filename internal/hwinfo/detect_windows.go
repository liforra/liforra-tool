//go:build windows

package hwinfo

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// psScript gathers everything in one PowerShell invocation (much faster
// than one process per WMI class) and emits JSON matching Info's shape
// directly, so no separate mapping step is needed on the Go side.
//
// Not yet exercised on real Windows hardware — built from well-known CIM
// cmdlets (Get-CimInstance/Get-PhysicalDisk), not live-verified. Treat
// field availability (especially MediaType/SerialNumber on odd storage
// controllers) as best-effort.
const psScript = `
$ErrorActionPreference = 'SilentlyContinue'
$cs = Get-CimInstance Win32_ComputerSystem
$bios = Get-CimInstance Win32_BIOS
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$os = Get-CimInstance Win32_OperatingSystem
$gpu = Get-CimInstance Win32_VideoController | Select-Object -First 1

$ddrTypeMap = @{ 20 = 'DDR'; 21 = 'DDR2'; 22 = 'DDR2 FB-DIMM'; 24 = 'DDR3'; 26 = 'DDR4'; 34 = 'DDR5' }
$formFactorMap = @{ 8 = 'DIMM'; 12 = 'SO-DIMM' }
$memory = Get-CimInstance Win32_PhysicalMemory | ForEach-Object {
    [PSCustomObject]@{
        capacityGB   = [math]::Round($_.Capacity / 1GB, 2)
        # ConfiguredClockSpeed is what the module is actually running at
        # right now (matches Task Manager) — Speed is only its SPD-rated
        # maximum, which a memory-controller-limited CPU or mixed-speed
        # kit can clock down from (confirmed live: a real device reported
        # Speed=2667 while actually running, and Task Manager agreeing, at
        # 2133). Fall back to Speed only if ConfiguredClockSpeed is 0 —
        # some older systems/drivers never populate it.
        speedMHz     = $(if ($_.ConfiguredClockSpeed) { $_.ConfiguredClockSpeed } else { $_.Speed })
        ddrType      = $ddrTypeMap[[int]$_.SMBIOSMemoryType]
        manufacturer = ("$($_.Manufacturer)").Trim()
        formFactor   = $formFactorMap[[int]$_.FormFactor]
    }
}

# Storage and network data come from their WMI classes directly — the
# Get-PhysicalDisk / Get-NetAdapter / Get-PnpDevice cmdlets return the same
# data but each costs 2-3s just to load its module in Windows PowerShell.
$busTypeMap = @{ 1 = 'SCSI'; 3 = 'ATA'; 7 = 'USB'; 8 = 'RAID'; 10 = 'SAS'; 11 = 'SATA'; 17 = 'NVMe' }
# This scan is about the machine's own internal storage — MSFT_PhysicalDisk
# enumerates every physical disk with no such filter, so a technician's own
# USB stick (plugged in to carry toolstar reports) would otherwise show up
# as an extra "disk" in the device, mislabeled as a generic HDD (it isn't
# SATA/NVMe and isn't reported as an SSD, so diskKind() falls through to
# "HDD" — confirmed live 2026-09-29 with a Ventoy stick plugged in).
$physicalDisks = @(Get-CimInstance -Namespace root/Microsoft/Windows/Storage -ClassName MSFT_PhysicalDisk | Where-Object { $_.BusType -ne 7 })
# Serial numbers from Win32_DiskDrive, keyed by index — a second, best-effort
# signal only (some USB/SATA bridges pad or byte-swap it), never required
# for the "does the wipe certificate name this drive" check to pass.
$serialByIndex = @{}
foreach ($d in (Get-CimInstance Win32_DiskDrive)) {
    $serialByIndex[[int]$d.Index] = ("$($d.SerialNumber)").Trim()
}
$disks = $physicalDisks | ForEach-Object {
    [PSCustomObject]@{
        # Decimal GB, the unit disks are sold in (a "256 GB" SSD is 256e9 bytes).
        capacityGB = [math]::Round($_.Size / 1e9, 2)
        isSSD      = ($_.MediaType -eq 4)
        busType    = $busTypeMap[[int]$_.BusType]
        model      = ("$($_.FriendlyName)").Trim()
        serial     = $(if ($serialByIndex.ContainsKey([int]$_.DeviceId)) { $serialByIndex[[int]$_.DeviceId] } else { '' })
    }
}

# Chassis type is a heuristic, not authoritative — see hwinfo.Info.DeviceType
# doc. Mapped straight to this GLPI's ComputerType names; anything unlisted
# (Desktop, Tower, Mini Tower, ...) is treated as a tower.
$typeByChassis = @{
    8 = 'Laptop'; 9 = 'Laptop'; 10 = 'Laptop'; 14 = 'Laptop'
    30 = 'Tablet'; 31 = 'Convertible'; 32 = 'Convertible'
    13 = 'All-in-One-PC'; 35 = 'Mini-PC'; 36 = 'Mini-PC'
    4 = 'SFF-Desktop (Small Form Factor)'; 15 = 'SFF-Desktop (Small Form Factor)'; 16 = 'SFF-Desktop (Small Form Factor)'
}
$deviceType = 'Desktop-PC (Tower)'
foreach ($c in (Get-CimInstance Win32_SystemEnclosure | Select-Object -First 1).ChassisTypes) {
    if ($typeByChassis.ContainsKey([int]$c)) { $deviceType = $typeByChassis[[int]$c]; break }
}

# Lenovo reports its machine-type code (e.g. 20HGA0QV00) as Model and the
# actual model name (e.g. ThinkPad T470s) as the product version.
$model = $cs.Model
$productVersion = ("$((Get-CimInstance Win32_ComputerSystemProduct).Version)").Trim()
if ($cs.Manufacturer -match 'lenovo' -and $productVersion) { $model = $productVersion }

# GLPI names Windows versions by release (25H2), not build (10.0.26200).
$cv = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
$osVersion = if ($cv.DisplayVersion) { $cv.DisplayVersion } elseif ($cv.ReleaseId) { $cv.ReleaseId } else { $os.Version }

$bb = Get-CimInstance Win32_BaseBoard
$mainboard = ("$($bb.Manufacturer) $($bb.Product)").Trim()

# NdisPhysicalMedium 9 = native 802.11.
$wlan = [bool](Get-CimInstance -Namespace root/StandardCimv2 -ClassName MSFT_NetAdapter -Filter "ConnectorPresent = TRUE AND NdisPhysicalMedium = 9")
$pnp = @(Get-CimInstance Win32_PnPEntity -Filter "PNPClass='Bluetooth' OR PNPClass='SDHost' OR Name LIKE '%Card Reader%'")
# Windows adds its own Bluetooth enumerators next to a real radio; only a
# vendor radio counts.
$bluetooth = [bool]($pnp | Where-Object { $_.PNPClass -eq 'Bluetooth' -and $_.Name -notmatch '^Microsoft|Enumerator|RFCOMM|Personal Area Network|LE Generic|Auflistung' })
$sdCardReader = [bool]($pnp | Where-Object { $_.PNPClass -eq 'SDHost' -or $_.Name -match 'card ?reader' })
$optical = Get-CimInstance Win32_CDROMDrive | Select-Object -First 1
$opticalDrive = if (-not $optical) { '' } elseif ("$($optical.Caption)" -match 'DVD|BD') { 'DVD' } else { 'CD' }
# Windows client licensing application id; status 1 = licensed.
$osActivated = [bool](Get-CimInstance SoftwareLicensingProduct -Filter "PartialProductKey IS NOT NULL AND ApplicationID='55c92734-d682-4d71-983e-d6ec3f16059f'" | Where-Object { $_.LicenseStatus -eq 1 })

# Physical wired NICs only. Win32_NetworkAdapter's AdapterType is useless
# here — on real hardware the Wi-Fi and Bluetooth PAN adapters both also
# report "Ethernet 802.3" — so this uses the same NdisPhysicalMedium signal
# as the WLAN check above (14 = native 802.3) and excludes USB-attached
# adapters (a plugged-in USB-Ethernet dongle isn't a built-in port of the
# machine). Verified 2026-09-29 against a live machine with an onboard NIC
# (PCI, counted) and a USB-Ethernet dongle (USB, excluded).
$ethernetPorts = @(Get-CimInstance -Namespace root/StandardCimv2 -ClassName MSFT_NetAdapter -Filter "NdisPhysicalMedium = 14" | Where-Object {
    $_.PnPDeviceID -notmatch '^USB'
}).Count

# BIOS release year as a stand-in for "Erscheinungsjahr" — not authoritative
# if the BIOS has since been updated, but there's no better source.
$releaseYear = if ($bios.ReleaseDate) { $bios.ReleaseDate.Year } else { 0 }

# Windows' own battery report, because WMI's BatteryStaticData (the design
# capacity) needs admin rights and fails silently for a normal user.
$batteries = @()
$batteryXml = Join-Path $env:TEMP ('liforra-battery-' + [guid]::NewGuid() + '.xml')
powercfg /batteryreport /xml /output $batteryXml | Out-Null
if (Test-Path $batteryXml) {
    [xml]$batteryReport = Get-Content $batteryXml -Raw
    Remove-Item $batteryXml
    $ns = @{ b = $batteryReport.DocumentElement.NamespaceURI }
    $batteries = @(Select-Xml -Xml $batteryReport -XPath '//b:Batteries/b:Battery' -Namespace $ns | ForEach-Object {
        [PSCustomObject]@{
            designCapacityMWh     = [int]$_.Node.DesignCapacity
            fullChargeCapacityMWh = [int]$_.Node.FullChargeCapacity
        }
    })
}

[PSCustomObject]@{
    manufacturer    = $cs.Manufacturer
    model           = $model
    serialNumber    = $bios.SerialNumber
    deviceType      = $deviceType
    cpuModel        = $cpu.Name
    memory          = @($memory)
    disks           = @($disks)
    gpuModel        = $gpu.Name
    osName          = $os.Caption
    osVersion       = $osVersion
    batteries       = @($batteries)
    mainboard       = $mainboard
    cpuMaxClockMHz  = [int]$cpu.MaxClockSpeed
    cpuCores        = [int]$cpu.NumberOfCores
    wlan            = $wlan
    bluetooth       = $bluetooth
    sdCardReader    = $sdCardReader
    opticalDrive    = $opticalDrive
    osActivated     = $osActivated
    ethernetPorts   = $ethernetPorts
    releaseYear     = $releaseYear
    diskNumbers     = @($physicalDisks | ForEach-Object { [int]$_.DeviceId })
} | ConvertTo-Json -Depth 5 -Compress
`

func Detect() (*Info, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// diskNumbers is parallel to disks: each one's \\.\PhysicalDriveN.
	var raw struct {
		Info
		DiskNumbers []int `json:"diskNumbers"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	info := raw.Info
	for i := range info.Disks {
		if i >= len(raw.DiskNumbers) {
			break
		}
		formFactor, protocol := probeDisk(raw.DiskNumbers[i])
		info.Disks[i].FormFactor = formFactor
		// Behind Intel RST in RAID mode the bus reads "RAID" even for a
		// single plain drive; the protocol the drive speaks still tells
		// SATA from NVMe.
		if bus := strings.ToLower(info.Disks[i].BusType); bus != "sata" && bus != "nvme" && protocol != "" {
			info.Disks[i].BusType = protocol
		}
	}
	info.Normalize()
	return &info, nil
}

// probeDisk asks Windows (IOCTL_STORAGE_QUERY_PROPERTY with
// StorageDevicePhysicalTopologyProperty) for the form factor and command
// protocol the drive reports in its own identify data — the only way to
// tell an M.2 or mSATA SATA SSD from a 2.5" one. Needs no admin rights.
// Returns "" for anything unknown or unavailable.
//
// Verified 2026-09-18 on a ThinkPad T470s: its SK hynix SC401 reports
// form factor M.2 and protocol ATA.
func probeDisk(diskNumber int) (formFactor, protocol string) {
	name, err := syscall.UTF16PtrFromString(fmt.Sprintf(`\\.\PhysicalDrive%d`, diskNumber))
	if err != nil {
		return "", ""
	}
	h, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return "", ""
	}
	defer syscall.CloseHandle(h)

	const (
		ioctlStorageQueryProperty             = 0x002D1400
		storageDevicePhysicalTopologyProperty = 54
	)
	query := make([]byte, 12) // STORAGE_PROPERTY_QUERY: PropertyId, QueryType (0 = standard), AdditionalParameters
	binary.LittleEndian.PutUint32(query, storageDevicePhysicalTopologyProperty)
	out := make([]byte, 4096)
	var n uint32
	if err := syscall.DeviceIoControl(h, ioctlStorageQueryProperty, &query[0], uint32(len(query)), &out[0], uint32(len(out)), &n, nil); err != nil {
		return "", ""
	}

	// STORAGE_PHYSICAL_TOPOLOGY_DESCRIPTOR has NodeCount at 8 and the first
	// STORAGE_PHYSICAL_NODE_DATA at 16, whose DeviceCount is at +16 and
	// DeviceDataOffset (relative to the node) at +24. In the
	// STORAGE_PHYSICAL_DEVICE_DATA it points to, CommandProtocol is at +12
	// and FormFactor at +20.
	const node = 16
	if n < node+28 || binary.LittleEndian.Uint32(out[8:]) == 0 || binary.LittleEndian.Uint32(out[node+16:]) == 0 {
		return "", ""
	}
	dev := node + int(binary.LittleEndian.Uint32(out[node+24:]))
	if dev+24 > int(n) {
		return "", ""
	}

	switch binary.LittleEndian.Uint32(out[dev+12:]) {
	case 2:
		protocol = "SATA"
	case 3:
		protocol = "NVMe"
	}
	switch binary.LittleEndian.Uint32(out[dev+20:]) {
	case 1:
		formFactor = "3.5"
	case 2:
		formFactor = "2.5"
	case 7:
		formFactor = "mSATA"
	case 8:
		formFactor = "M.2"
	}
	return formFactor, protocol
}
