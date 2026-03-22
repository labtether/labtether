//go:build darwin

package sysconfig

import "testing"

func TestParseDarwinNetstatOutput(t *testing.T) {
	raw := `Name  Mtu   Network       Address            Ipkts Ierrs    Opkts Oerrs  Coll  Drop     Ibytes    Obytes
en0   1500  <Link#4>      ac:de:48:00:11:22 12345     0    67890     0     0     0    12345678  9876543
`

	rxBytes, txBytes, rxPackets, txPackets, ok := ParseDarwinNetstatOutput("en0", raw)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if rxBytes != 12345678 || txBytes != 9876543 {
		t.Fatalf("unexpected bytes rx=%d tx=%d", rxBytes, txBytes)
	}
	if rxPackets != 12345 || txPackets != 67890 {
		t.Fatalf("unexpected packets rx=%d tx=%d", rxPackets, txPackets)
	}
}

func TestParseDarwinNetstatOutputSkipsDashRows(t *testing.T) {
	raw := `Name  Mtu   Network       Address            Ipkts Ierrs    Opkts Oerrs  Coll  Drop     Ibytes    Obytes
en0   1500  <Link#4>      ac:de:48:00:11:22 12345     0    67890     0     0     0    12345678  9876543
en0   1500  192.168.1     192.168.1.10      -         -    -         -     -     -    -         -
`

	rxBytes, txBytes, rxPackets, txPackets, ok := ParseDarwinNetstatOutput("en0", raw)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if rxBytes != 12345678 || txBytes != 9876543 || rxPackets != 12345 || txPackets != 67890 {
		t.Fatalf("unexpected parsed counters: rxBytes=%d txBytes=%d rxPackets=%d txPackets=%d", rxBytes, txBytes, rxPackets, txPackets)
	}
}

func TestParseDarwinNetstatOutputMissingHeader(t *testing.T) {
	_, _, _, _, ok := ParseDarwinNetstatOutput("en0", "invalid")
	if ok {
		t.Fatalf("expected parse failure")
	}
}
