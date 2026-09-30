// Package specsheet generates the "pc_info<device-name>.txt" spec sheet —
// the exact template format was reverse-engineered from AWO's existing
// (non-GLPI) tool, see project memory [[finished_device_txt_format]].
package specsheet

import (
	"strconv"
	"strings"
)

type Ports struct {
	VGA         int `json:"vga"`
	DisplayPort int `json:"displayPort"`
	HDMI        int `json:"hdmi"`
	DVI         int `json:"dvi"`
	USB3        int `json:"usb3"`
	USB2        int `json:"usb2"`
	Ethernet    int `json:"ethernet"`
	Kopfhoerer  int `json:"kopfhoerer"`
	Mikrofon    int `json:"mikrofon"`
	LineIn      int `json:"lineIn"`
	PS2         int `json:"ps2"`
}

type Anwendungsgebiete struct {
	Bueroarbeit    bool `json:"bueroarbeit"`
	Gaming         bool `json:"gaming"`
	Multimedia     bool `json:"multimedia"`
	Programmierung bool `json:"programmierung"`
	GrafikDesign   bool `json:"grafikDesign"`
}

type Lieferumfang struct {
	PCSystem         bool `json:"pcSystem"`
	Kabel            bool `json:"kabel"`
	Windows11Digital bool `json:"windows11Digital"`
	Windows11Key     bool `json:"windows11Key"`
}

type Fields struct {
	SKU        string `json:"sku"`
	Geraetetyp string `json:"geraetetyp"` // "Desktop" | "Laptop"

	Marke                    string `json:"marke"`
	Modell                   string `json:"modell"`
	Produktart               string `json:"produktart"`
	Mainboard                string `json:"mainboard"`
	Prozessor                string `json:"prozessor"`
	Prozessorgeschwindigkeit string `json:"prozessorgeschwindigkeit"`
	RAM                      string `json:"ram"`
	RAMGeschwindigkeit       string `json:"ramGeschwindigkeit"`
	SSD                      string `json:"ssd"`
	Sekundaerspeicher        string `json:"sekundaerspeicher"`
	Grafikeinheit            string `json:"grafikeinheit"`
	Betriebssystem           string `json:"betriebssystem"`
	Erscheinungsjahr         string `json:"erscheinungsjahr"`
	Farbe                    string `json:"farbe"`

	WLAN               bool   `json:"wlan"`
	Bluetooth          bool   `json:"bluetooth"`
	Laufwerk           string `json:"laufwerk"`
	SDKartensteckplatz bool   `json:"sdKartensteckplatz"`

	Ports Ports `json:"ports"`

	Gewicht     string `json:"gewicht"`
	Abmessungen string `json:"abmessungen"`

	Anwendungsgebiete Anwendungsgebiete `json:"anwendungsgebiete"`
	Lieferumfang      Lieferumfang      `json:"lieferumfang"`

	Bemerkungen string `json:"bemerkungen"`
}

func vorhanden(b bool) string {
	if b {
		return "Vorhanden"
	}
	return "Nicht vorhanden"
}

func check(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

// Generate produces the exact TXT spec-sheet format of the ones already in
// GLPI (checked 2026-09-18 against the 458 real ones, which are
// authoritative): every field is printed even when empty ("» Laufwerk: "),
// section headers stay even with nothing under them, CRLF line endings, a
// trailing line break, and a "Bemerkungen" section only when there is one.
func Generate(f Fields) string {
	var out []string

	out = append(out, "SKU (Artikelnummer): "+f.SKU)
	out = append(out, "Gerätetyp: "+f.Geraetetyp)
	out = append(out, "")

	out = append(out, "PC-System")
	out = append(out, "")
	out = append(out, "Systemkonfiguration")
	appendField(&out, "Marke", f.Marke)
	appendField(&out, "Modell", f.Modell)
	appendField(&out, "Produktart", f.Produktart)
	appendField(&out, "Mainboard", f.Mainboard)
	appendField(&out, "Prozessor", f.Prozessor)
	appendField(&out, "Prozessorgeschwindigkeit", f.Prozessorgeschwindigkeit)
	appendField(&out, "Arbeitsspeicher (RAM)", f.RAM)
	appendField(&out, "Arbeitsspeicher RAM Geschwindigkeit", f.RAMGeschwindigkeit)
	appendField(&out, "Systemspeicher (SSD)", f.SSD)
	appendField(&out, "Sekundärer Speicher (HDD/SSD)", f.Sekundaerspeicher)
	appendField(&out, "Grafikeinheit", f.Grafikeinheit)
	appendField(&out, "Betriebssystem", f.Betriebssystem)
	appendField(&out, "Erscheinungsjahr", f.Erscheinungsjahr)
	appendField(&out, "Farbe", f.Farbe)

	out = append(out, "")
	out = append(out, "Konnektivität")
	out = append(out, "» WLAN: "+vorhanden(f.WLAN))
	out = append(out, "» Bluetooth: "+vorhanden(f.Bluetooth))
	appendField(&out, "Laufwerk", f.Laufwerk)
	out = append(out, "» SD-Kartensteckplatz: "+strings.ToLower(vorhanden(f.SDKartensteckplatz)))

	out = append(out, "")
	out = append(out, "Anschlüsse")
	appendPort(&out, f.Ports.VGA, "VGA")
	appendPort(&out, f.Ports.DisplayPort, "DisplayPort")
	appendPort(&out, f.Ports.HDMI, "HDMI")
	appendPort(&out, f.Ports.DVI, "DVI")
	appendPort(&out, f.Ports.USB3, "USB 3.0/3.1 Gen 1 (Typ-A)")
	appendPort(&out, f.Ports.USB2, "USB 2.0 (Typ-A)")
	appendPort(&out, f.Ports.Ethernet, "Ethernet (RJ-45)")
	appendPort(&out, f.Ports.Kopfhoerer, "Kopfhörer-Ausgang (3,5 mm Klinke)")
	appendPort(&out, f.Ports.Mikrofon, "Mikrofon-Eingang (3,5 mm Klinke)")
	appendPort(&out, f.Ports.LineIn, "Line-In (3,5 mm Klinke)")
	appendPort(&out, f.Ports.PS2, "PS/2")

	out = append(out, "")
	out = append(out, "Gewicht und Größe")
	appendField(&out, "Gewicht", f.Gewicht)
	appendField(&out, "Abmessungen (BxHxT)", f.Abmessungen)

	out = append(out, "")
	out = append(out, "Anwendungsgebiete")
	out = append(out, "» Büroarbeit: "+check(f.Anwendungsgebiete.Bueroarbeit))
	out = append(out, "» Gaming: "+check(f.Anwendungsgebiete.Gaming))
	out = append(out, "» Multimedia: "+check(f.Anwendungsgebiete.Multimedia))
	out = append(out, "» Programmierung: "+check(f.Anwendungsgebiete.Programmierung))
	out = append(out, "» Grafik- und Videodesign: "+check(f.Anwendungsgebiete.GrafikDesign))

	out = append(out, "")
	out = append(out, "Lieferumfang")
	if f.Lieferumfang.PCSystem {
		out = append(out, "» PC-System")
	}
	if f.Lieferumfang.Kabel {
		out = append(out, "» Stromkabel für das PC-System")
	}
	if f.Lieferumfang.Windows11Digital {
		out = append(out, "» Windows 11 Pro (Digital aktiviert)")
	}
	if f.Lieferumfang.Windows11Key {
		out = append(out, "» Windows 11 Pro (geschriebener Produkt Key)")
	}

	if strings.TrimSpace(f.Bemerkungen) != "" {
		out = append(out, "")
		out = append(out, "Bemerkungen")
		out = append(out, strings.TrimSpace(f.Bemerkungen))
	}

	return strings.Join(out, "\r\n") + "\r\n"
}

func appendField(out *[]string, label, value string) {
	*out = append(*out, "» "+label+": "+strings.TrimSpace(value))
}

func appendPort(out *[]string, count int, label string) {
	if count <= 0 {
		return
	}
	*out = append(*out, formatPortLine(count, label))
}

func formatPortLine(count int, label string) string {
	return "» " + strconv.Itoa(count) + "x " + label
}
