package specsheet

import "testing"

// pc_info#2584.txt as stored in GLPI (2026-09-18), byte for byte — except
// "SD-Kartensteckplatz: vorhanden", which this one sheet capitalizes while
// most real sheets (and the generator) write it lowercase.
const realSheet2584 = "SKU (Artikelnummer): #2584\r\n" +
	"Gerätetyp: Laptop\r\n" +
	"\r\n" +
	"PC-System\r\n" +
	"\r\n" +
	"Systemkonfiguration\r\n" +
	"» Marke: Lenovo\r\n" +
	"» Modell: Thinkpad T470p\r\n" +
	"» Produktart: Laptop\r\n" +
	"» Mainboard: Lenovo 20J6S02400\r\n" +
	"» Prozessor: Intel Core i7-7700 HQ\r\n" +
	"» Prozessorgeschwindigkeit: 2,8 GHz\r\n" +
	"» Arbeitsspeicher (RAM): 32 GB\r\n" +
	"» Arbeitsspeicher RAM Geschwindigkeit: 2400 MHz\r\n" +
	"» Systemspeicher (SSD): 512 GB NVMe\r\n" +
	"» Sekundärer Speicher (HDD/SSD): Nicht Vorhanden\r\n" +
	"» Grafikeinheit: NVIDIA GeForce 940MX\r\n" +
	"» Betriebssystem: Windows 11 Pro\r\n" +
	"» Erscheinungsjahr: ca. 2017\r\n" +
	"» Farbe: Schwarz\r\n" +
	"\r\n" +
	"Konnektivität\r\n" +
	"» WLAN: Vorhanden\r\n" +
	"» Bluetooth: Vorhanden\r\n" +
	"» Laufwerk: Nicht Vorhanden\r\n" +
	"» SD-Kartensteckplatz: vorhanden\r\n" +
	"\r\n" +
	"Anschlüsse\r\n" +
	"» 1x HDMI\r\n" +
	"» 3x USB 2.0 (Typ-A)\r\n" +
	"» 1x Ethernet (RJ-45)\r\n" +
	"» 1x Line-In (3,5 mm Klinke)\r\n" +
	"\r\n" +
	"Gewicht und Größe\r\n" +
	"» Gewicht: 1,7 KG\r\n" +
	"» Abmessungen (BxHxT): 339 x 24 x 235 mm\r\n" +
	"\r\n" +
	"Anwendungsgebiete\r\n" +
	"» Büroarbeit: ✓\r\n" +
	"» Gaming: ✗\r\n" +
	"» Multimedia: ✓\r\n" +
	"» Programmierung: ✓\r\n" +
	"» Grafik- und Videodesign: ✓\r\n" +
	"\r\n" +
	"Lieferumfang\r\n" +
	"» PC-System\r\n" +
	"» Stromkabel für das PC-System\r\n" +
	"» Windows 11 Pro (Digital aktiviert)\r\n" +
	"\r\n" +
	"Bemerkungen\r\n" +
	"Mini Dp Anschluss Vorhanden\r\n"

func TestGenerate_MatchesRealSheetByteForByte(t *testing.T) {
	got := Generate(Fields{
		SKU:                      "#2584",
		Geraetetyp:               "Laptop",
		Marke:                    "Lenovo",
		Modell:                   "Thinkpad T470p",
		Produktart:               "Laptop",
		Mainboard:                "Lenovo 20J6S02400",
		Prozessor:                "Intel Core i7-7700 HQ",
		Prozessorgeschwindigkeit: "2,8 GHz",
		RAM:                      "32 GB",
		RAMGeschwindigkeit:       "2400 MHz",
		SSD:                      "512 GB NVMe",
		Sekundaerspeicher:        "Nicht Vorhanden",
		Grafikeinheit:            "NVIDIA GeForce 940MX",
		Betriebssystem:           "Windows 11 Pro",
		Erscheinungsjahr:         "ca. 2017",
		Farbe:                    "Schwarz",
		WLAN:                     true,
		Bluetooth:                true,
		Laufwerk:                 "Nicht Vorhanden",
		SDKartensteckplatz:       true,
		Ports:                    Ports{HDMI: 1, USB2: 3, Ethernet: 1, LineIn: 1},
		Gewicht:                  "1,7 KG",
		Abmessungen:              "339 x 24 x 235 mm",
		Anwendungsgebiete:        Anwendungsgebiete{Bueroarbeit: true, Multimedia: true, Programmierung: true, GrafikDesign: true},
		Lieferumfang:             Lieferumfang{PCSystem: true, Kabel: true, Windows11Digital: true},
		Bemerkungen:              "Mini Dp Anschluss Vorhanden",
	})
	if got != realSheet2584 {
		t.Errorf("generated sheet differs from the real one\n--- got ---\n%q\n--- want ---\n%q", got, realSheet2584)
	}
}
