// Package hwinfo detects the specs of the machine the app is currently
// running on — the live counterpart to parsing a toolstar report from a USB
// stick (see internal/toolstar), for devices being freshly added where no
// report exists yet.
package hwinfo

import "strings"

type Disk struct {
	CapacityGB float64 `json:"capacityGB"`
	IsSSD      bool    `json:"isSSD"`
	// BusType as Windows reports it, e.g. "SATA", "NVMe", "USB".
	BusType string `json:"busType"`
	// FormFactor as the drive itself reports it: "2.5", "3.5", "M.2",
	// "mSATA", or empty if it doesn't say.
	FormFactor string `json:"formFactor"`
	// Kind names the drive the way this GLPI's DeviceHardDrive catalog does
	// ("SATA SSD", "M.2 SATA SSD", "mSATA SSD", "M.2 NVME SSD", "SATA HDD")
	// — derived in Normalize, and correctable by the technician.
	Kind string `json:"kind"`
	// Model is the drive's own reported model string (e.g. "Samsung SSD
	// 850 EVO 250GB") — not shown anywhere, used only by
	// internal/devicecheck to confirm an erase certificate names the disk
	// actually installed. Empty if unavailable.
	Model string `json:"model"`
	// Serial is best-effort — some USB/SATA bridges return it padded or
	// byte-swapped, so it's a secondary signal, never required for a match.
	Serial string `json:"serial"`
}

func diskKind(d Disk) string {
	switch strings.ToLower(d.BusType) {
	case "nvme":
		return "M.2 NVME SSD"
	case "sata":
		if !d.IsSSD {
			return "SATA HDD"
		}
		switch d.FormFactor {
		case "M.2":
			return "M.2 SATA SSD"
		case "mSATA":
			return "mSATA SSD"
		}
		return "SATA SSD"
	}
	if d.IsSSD {
		return "SSD"
	}
	return "HDD"
}

type MemoryModule struct {
	CapacityGB   float64 `json:"capacityGB"`
	SpeedMHz     int     `json:"speedMHz"`
	DDRType      string  `json:"ddrType"`
	Manufacturer string  `json:"manufacturer"`
	// FormFactor is "DIMM", "SO-DIMM", or empty if not reported.
	FormFactor string `json:"formFactor"`
}

type Battery struct {
	DesignCapacityMWh     int `json:"designCapacityMWh"`
	FullChargeCapacityMWh int `json:"fullChargeCapacityMWh"`
}

type Info struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	SerialNumber string `json:"serialNumber"`
	// DeviceType is a best-effort guess from the chassis type reported by
	// the system, already phrased as this GLPI's ComputerType names
	// ("Laptop", "Desktop-PC (Tower)", "SFF-Desktop (Small Form Factor)",
	// ...) — not authoritative, a technician should confirm/correct it.
	DeviceType string         `json:"deviceType"`
	CPUModel   string         `json:"cpuModel"`
	Memory     []MemoryModule `json:"memory"`
	Disks      []Disk         `json:"disks"`
	GPUModel   string         `json:"gpuModel"`
	OSName     string         `json:"osName"`
	OSVersion  string         `json:"osVersion"`
	// Batteries is empty on desktops. Health % (design vs. full-charge
	// capacity) feeds the "Akkugesundheit" Zusatzdaten field.
	Batteries []Battery `json:"batteries"`

	// The rest only feeds the spec-sheet TXT and the "Betriebssystem
	// aktiviert" Zusatzdaten field.

	// Mainboard is "<manufacturer> <product>", e.g. "LENOVO 20HGA0QV00" —
	// the format AWO's existing spec sheets use.
	Mainboard string `json:"mainboard"`
	// CPUMaxClockMHz is a fallback for CPUs whose name doesn't state the
	// base clock ("... @ 2.50GHz"); it can report turbo instead of base.
	CPUMaxClockMHz int  `json:"cpuMaxClockMHz"`
	WLAN           bool `json:"wlan"`
	Bluetooth      bool `json:"bluetooth"`
	SDCardReader   bool `json:"sdCardReader"`
	// OpticalDrive is "DVD", "CD", or empty if there is none.
	OpticalDrive string `json:"opticalDrive"`
	OSActivated  bool   `json:"osActivated"`

	// CPUCores is physical core count — used only to guess at
	// "Anwendungsgebiete" (Programmierung/Grafik-Design), never shown on its
	// own.
	CPUCores int `json:"cpuCores"`
	// EthernetPorts counts physical wired network adapters (not Wi-Fi,
	// Bluetooth PAN, or virtual/VPN adapters) — the one "Anschlüsse" count
	// that's actually detectable on Windows. VGA/DisplayPort/HDMI/DVI,
	// headphone/microphone/line-in and PS/2 have no such signal (Windows
	// only knows about a currently-connected monitor or audio device, never
	// an empty port), and USB port counts would need undocumented Windows
	// Driver Kit IOCTLs this package doesn't attempt — left for the
	// technician to fill in.
	EthernetPorts int `json:"ethernetPorts"`
	// ReleaseYear is the BIOS's own release year (Win32_BIOS.ReleaseDate) —
	// a reasonable stand-in for "Erscheinungsjahr" on a machine that hasn't
	// had a later BIOS update change it. 0 if unknown.
	ReleaseYear int `json:"releaseYear"`
}
