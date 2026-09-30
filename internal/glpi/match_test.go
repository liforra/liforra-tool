package glpi

import "testing"

// Candidate names are copied from the live GLPI instance (2026-09-18);
// scanned values are what Windows reports for real hardware.
func TestBestMatch(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		hints   []string
		field   string
		entries []string
		want    int // 1-based index into entries, 0 = no match
	}{
		{
			name:    "CPU: trademarks, clock speed and 'CPU' ignored",
			value:   "Intel(R) Core(TM) i5-7200U CPU @ 2.50GHz",
			field:   "designation",
			entries: []string{"Core i5-7300U", "Core i5-7200U"},
			want:    2,
		},
		{
			name:    "CPU: prefers the entry explaining more of the scan",
			value:   "Intel(R) Core(TM) i5-7200U CPU @ 2.50GHz",
			field:   "designation",
			entries: []string{"Core i5-7200U", "Intel Core i5 7200U"},
			want:    2,
		},
		{
			name:    "CPU: 11th gen prefix dropped",
			value:   "11th Gen Intel(R) Core(TM) i5-1135G7 @ 2.40GHz",
			field:   "designation",
			entries: []string{"Intel Core i5 1145G7", "Intel Core i5 1135G7"},
			want:    2,
		},
		{
			name:    "CPU: different model number never matches",
			value:   "Intel(R) Core(TM) i5-7200U CPU @ 2.50GHz",
			field:   "designation",
			entries: []string{"Intel Core i5", "Intel Core i7-7200U"},
			want:    0,
		},
		{
			name:    "GPU: HD vs UHD kept apart",
			value:   "Intel(R) HD Graphics 620",
			field:   "designation",
			entries: []string{"Intel UHD Graphics 620", "Intel HD Graphics 620", "Intel HD Graphics 630"},
			want:    2,
		},
		{
			name:    "GPU: Ti is its own model",
			value:   "NVIDIA GeForce GTX 1050",
			field:   "designation",
			entries: []string{"NVIDIA GeForce GTX 1050 Ti", "NVIDIA GeForce GTX 1050"},
			want:    2,
		},
		{
			name:    "OS: vendor and edition ignored",
			value:   "Microsoft Windows 11 Pro",
			field:   "name",
			entries: []string{"Windows 10", "Windows 11", "Windows Server 2022"},
			want:    2,
		},
		{
			name:    "OS: server year must match",
			value:   "Microsoft Windows Server 2019 Standard",
			field:   "name",
			entries: []string{"Windows Server 2016", "Windows Server 2019"},
			want:    2,
		},
		{
			name:    "model: manufacturer hint picks the prefixed duplicate",
			value:   "ThinkPad T470",
			hints:   []string{"LENOVO"},
			field:   "name",
			entries: []string{"Thinkpad T470", "Lenovo ThinkPad T470"},
			want:    2,
		},
		{
			name:    "model: suffix letter is part of the model",
			value:   "ThinkPad T470s",
			hints:   []string{"LENOVO"},
			field:   "name",
			entries: []string{"Lenovo ThinkPad T470", "Lenovo ThinkPad T470p", "Lenovo ThinkPad T470s"},
			want:    3,
		},
		{
			name:    "model: Dell reports it without the brand",
			value:   "Latitude 7490",
			hints:   []string{"Dell Inc."},
			field:   "name",
			entries: []string{"Dell Latitude 7480", "Dell Latitude 7490"},
			want:    2,
		},
		{
			name:    "model: generation number must match",
			value:   "ThinkPad P1 Gen 2",
			hints:   []string{"LENOVO"},
			field:   "name",
			entries: []string{"ThinkPad P1", "ThinkPad P1 Gen 1", "ThinkPad P1 Gen 2"},
			want:    3,
		},
		{
			name:    "manufacturer: case and company suffix ignored",
			value:   "Dell Inc.",
			field:   "name",
			entries: []string{"Dell EMC", "Dell"},
			want:    2,
		},
		{
			name:    "manufacturer: Hewlett-Packard is HP, not HPE",
			value:   "Hewlett-Packard",
			field:   "name",
			entries: []string{"HPE (Hewlett Packard Enterprise)", "HP"},
			want:    2,
		},
		{
			name:    "manufacturer: long legal name",
			value:   "FUJITSU CLIENT COMPUTING LIMITED",
			field:   "name",
			entries: []string{"Aqua Computer", "Fujitsu"},
			want:    2,
		},
		{
			name:    "type: no partial word match",
			value:   "Laptop",
			field:   "name",
			entries: []string{"Gaming-Laptop", "Laptop"},
			want:    2,
		},
		{
			name:    "type: ambiguous generic word matches nothing",
			value:   "Desktop",
			field:   "name",
			entries: []string{"Desktop-PC (Tower)", "SFF-Desktop (Small Form Factor)"},
			want:    0,
		},
		{
			name:    "memory: SO-DIMM is not DIMM",
			value:   "SO-DIMM DDR4 4GB 2133MHz",
			field:   "designation",
			entries: []string{"DIMM DDR4 4GB 2133MHz", "SO-DIMM DDR4 4GB 2133MHz"},
			want:    2,
		},
		{
			name:    "memory: SO-DIMM never falls back to DIMM",
			value:   "SO-DIMM DDR4 4GB 2133MHz",
			field:   "designation",
			entries: []string{"DIMM DDR4 4GB 2133MHz"},
			want:    0,
		},
		{
			name:    "memory: spaced units and word order",
			value:   "DIMM DDR3 8GB 1600MHz",
			field:   "designation",
			entries: []string{"DIMM 8 GB DDR3 1600 MHz"},
			want:    1,
		},
		{
			name:    "disk: lowest id among exact duplicates",
			value:   "SATA SSD 256GB",
			field:   "designation",
			entries: []string{"M.2 SATA SSD 256GB", "SATA SSD 256GB", "SATA SSD 256GB", "mSATA SSD 256GB"},
			want:    2,
		},
		{
			name:    "disk: M.2 SATA is not plain SATA",
			value:   "M.2 SATA SSD 256GB",
			field:   "designation",
			entries: []string{"SATA SSD 256GB", "M.2 SATA SSD 256GB", "mSATA SSD 256GB"},
			want:    2,
		},
		{
			name:    "disk: NVMe",
			value:   "M.2 NVME SSD 512GB",
			field:   "designation",
			entries: []string{"M.2 SATA SSD 512GB", "M.2 NVME SSD 512GB"},
			want:    2,
		},
		{
			name:    "disk: capacity must match",
			value:   "SATA SSD 512GB",
			field:   "designation",
			entries: []string{"SATA SSD 256GB"},
			want:    0,
		},
		{
			name:    "empty value matches nothing",
			value:   "",
			field:   "name",
			entries: []string{"Windows 11"},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entries []namedEntry
			for i, label := range tt.entries {
				e := namedEntry{ID: i + 1}
				if tt.field == "designation" {
					e.Designation = label
				} else {
					e.Name = label
				}
				entries = append(entries, e)
			}
			if got := bestMatch(tt.value, tt.hints, entries, tt.field); got != tt.want {
				t.Errorf("bestMatch(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestSearchTokenAppearsVerbatim(t *testing.T) {
	tests := map[string]string{
		"Intel(R) Core(TM) i5-7200U CPU @ 2.50GHz": "7200u",
		"Microsoft Windows 11 Pro":                  "11",
		"SO-DIMM DDR4 4GB 2133MHz":                  "ddr4",
		"SATA SSD 256GB":                            "",
		"LENOVO":                                    "",
	}
	for value, want := range tests {
		if got := searchToken(canonicalTokens(value)); got != want {
			t.Errorf("searchToken(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestDesignations(t *testing.T) {
	if got := diskDesignation(DiskInput{Kind: "M.2 SATA SSD", CapacityGB: 256}); got != "M.2 SATA SSD 256GB" {
		t.Errorf("M.2 SATA SSD: got %q", got)
	}
	if got := diskDesignation(DiskInput{Kind: "M.2 NVME SSD", CapacityGB: 512}); got != "M.2 NVME SSD 512GB" {
		t.Errorf("NVMe: got %q", got)
	}
	if got := diskDesignation(DiskInput{Kind: "SATA HDD", CapacityGB: 1000}); got != "SATA HDD 1TB" {
		t.Errorf("SATA HDD 1TB: got %q", got)
	}
	if got := diskDesignation(DiskInput{Kind: "SATA HDD", CapacityGB: 2048}); got != "SATA HDD 2TB" {
		t.Errorf("SATA HDD 2TB: got %q", got)
	}
	if got := memoryDesignation(MemoryModuleInput{FormFactor: "SO-DIMM", DDRType: "DDR4", CapacityGB: 16, SpeedMHz: 2133}, false); got != "SO-DIMM DDR4 16GB 2133MHz" {
		t.Errorf("reported SO-DIMM: got %q", got)
	}
	if got := memoryDesignation(MemoryModuleInput{DDRType: "DDR4", CapacityGB: 8, SpeedMHz: 2400}, true); got != "SO-DIMM DDR4 8GB 2400MHz" {
		t.Errorf("laptop fallback: got %q", got)
	}
}
