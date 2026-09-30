package hwinfo

import "testing"

func TestNormalize_DiskCapacityInDecimalGB(t *testing.T) {
	info := Info{Disks: []Disk{
		{CapacityGB: 256.06, IsSSD: true, BusType: "SATA", FormFactor: "M.2"}, // SK hynix SC401 256GB, T470s
		{CapacityGB: 240.06, IsSSD: true, BusType: "SATA"},
		{CapacityGB: 512.11, IsSSD: true, BusType: "NVMe"},
		{CapacityGB: 1000.2, BusType: "SATA"},
	}}
	info.Normalize()

	want := []struct {
		capacity float64
		kind     string
	}{
		{256, "M.2 SATA SSD"},
		{240, "SATA SSD"},
		{512, "M.2 NVME SSD"},
		{1000, "SATA HDD"},
	}
	for i, w := range want {
		if got := info.Disks[i].CapacityGB; got != w.capacity {
			t.Errorf("disk %d: capacity %v, want %v", i, got, w.capacity)
		}
		if got := info.Disks[i].Kind; got != w.kind {
			t.Errorf("disk %d: kind %q, want %q", i, got, w.kind)
		}
	}
}
