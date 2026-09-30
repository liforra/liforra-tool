package hwinfo

// Raw OS-reported capacities rarely land exactly on the actual marketed
// size: RAM loses a slice to reserved regions (a physical 20GB kit — e.g.
// 4GB soldered + a 16GB SO-DIMM, a common laptop config — reports as only
// ~19.3GB usable), and disks are a little over or under their label. So
// detected capacities get snapped to the closest marketed value.
//
// Disk capacities must be in decimal GB (bytes / 1e9) before snapping —
// the unit disks are sold in. Binary GiB is ~7% smaller and lands on the
// wrong size when two are close: a 256 GB SSD is 238.4 GiB, nearer to 240
// than to 256 (seen live on a ThinkPad T470s, 2026-09-18).
//
// The RAM list is generated from sums of one or two standard module sizes
// (covers single-stick and dual-channel/asymmetric configs like the 4+16
// example above) rather than hand-picked, so a real total can't be missing.
var commonRAMSizesGB = generateRAMSizeCandidates()

func generateRAMSizeCandidates() []float64 {
	moduleSizes := []float64{1, 2, 4, 8, 16, 32, 64, 128}
	seen := map[float64]bool{}
	var sizes []float64
	add := func(v float64) {
		if !seen[v] {
			seen[v] = true
			sizes = append(sizes, v)
		}
	}
	for _, a := range moduleSizes {
		add(a)
		for _, b := range moduleSizes {
			add(a + b)
		}
	}
	return sizes
}

var commonDiskSizesGB = []float64{
	16, 32, 60, 64, 120, 128, 240, 250, 256, 320, 480, 500, 512,
	750, 960, 1000, 1024, 2000, 2048, 4000, 4096, 8000, 8192,
}

func snapToNearest(value float64, candidates []float64) float64 {
	if value <= 0 {
		return value
	}
	best := candidates[0]
	bestDiff := absF(value - best)
	for _, c := range candidates[1:] {
		if d := absF(value - c); d < bestDiff {
			best, bestDiff = c, d
		}
	}
	return best
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// Normalize snaps memory/disk capacities to the nearest common real-world
// size and derives each disk's Kind. Call once after gathering raw
// detection results.
func (info *Info) Normalize() {
	for i := range info.Memory {
		info.Memory[i].CapacityGB = snapToNearest(info.Memory[i].CapacityGB, commonRAMSizesGB)
	}
	for i := range info.Disks {
		info.Disks[i].CapacityGB = snapToNearest(info.Disks[i].CapacityGB, commonDiskSizesGB)
		info.Disks[i].Kind = diskKind(info.Disks[i])
	}
}
