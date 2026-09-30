//go:build linux

package hwinfo

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Detect is a dev-only convenience so the hardware-detection flow can be
// exercised on this Linux dev machine — see project memory. Uses /proc and
// common CLI tools instead of WMI; not meant to ship (the real build only
// ever compiles detect_windows.go).
func Detect() (*Info, error) {
	info := &Info{}

	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "model name") {
				if i := strings.Index(line, ":"); i != -1 {
					info.CPUModel = strings.TrimSpace(line[i+1:])
				}
				break
			}
		}
	}

	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, _ := strconv.ParseFloat(fields[1], 64)
					info.Memory = append(info.Memory, MemoryModule{CapacityGB: kb / 1024 / 1024})
				}
				break
			}
		}
	}

	if out, err := exec.Command("lsblk", "-J", "-d", "-o", "MODEL,SIZE,ROTA,TRAN").Output(); err == nil {
		var result struct {
			BlockDevices []struct {
				Model string `json:"model"`
				Size  string `json:"size"`
				Rota  bool   `json:"rota"`
				Tran  string `json:"tran"`
			} `json:"blockdevices"`
		}
		if json.Unmarshal(out, &result) == nil {
			for _, d := range result.BlockDevices {
				// No model means a virtual device (loop, zram), not a disk.
				if d.Model == "" {
					continue
				}
				info.Disks = append(info.Disks, Disk{
					CapacityGB: parseLsblkSize(d.Size) * 1.073741824, // GiB -> GB
					IsSSD:      !d.Rota,
					BusType:    d.Tran,
				})
			}
		}
	}

	if out, err := exec.Command("sh", "-c", "lspci | grep -i VGA").Output(); err == nil {
		line := strings.TrimSpace(string(out))
		if i := strings.Index(line, ": "); i != -1 {
			info.GPUModel = strings.TrimSpace(line[i+2:])
		}
	}

	info.DeviceType = "Desktop-PC (Tower)"
	if entries, err := os.ReadDir("/sys/class/power_supply"); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "BAT") {
				info.DeviceType = "Laptop"
				base := "/sys/class/power_supply/" + e.Name() + "/"
				design := readIntFile(base + "energy_full_design")
				full := readIntFile(base + "energy_full")
				info.Batteries = append(info.Batteries, Battery{DesignCapacityMWh: design, FullChargeCapacityMWh: full})
			}
		}
	}

	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.OSName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
	}

	info.Normalize()
	return info, nil
}

func readIntFile(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	// energy_full* is in µWh; convert to mWh to match the Windows side.
	return n / 1000
}

// parseLsblkSize converts lsblk's human-readable SIZE column (e.g. "238.5G",
// "1.8T" — binary units) to binary gigabytes; the caller converts to
// decimal, see normalize.go.
func parseLsblkSize(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	unit := s[len(s)-1]
	numPart := s[:len(s)-1]
	n, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case 'T':
		return n * 1024
	case 'G':
		return n
	case 'M':
		return n / 1024
	}
	return 0
}
