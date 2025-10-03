package dbmate

import (
	"bytes"
	"testing"
)

func TestComputeChecksum_LFvsCRLF(t *testing.T) {
	a := []byte("-- migrate:up\nCREATE TABLE foo (id INTEGER);\n")
	b := []byte("-- migrate:up\r\nCREATE TABLE foo (id INTEGER);\r\n")
	ha := ComputeChecksum(a)
	hb := ComputeChecksum(b)
	if ha != hb {
		t.Fatalf("checksums differ for LF vs CRLF: %s != %s", ha, hb)
	}
}

func TestComputeChecksum_BOMStripped(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	body := []byte("-- migrate:up\nCREATE TABLE foo (id INTEGER);\n")
	withBOM := append(bom, body...)
	h1 := ComputeChecksum(body)
	h2 := ComputeChecksum(withBOM)
	if h1 != h2 {
		t.Fatalf("checksums differ with/without BOM: %s != %s", h1, h2)
	}
}

func TestComputeChecksum_CRLFandBOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	lf := []byte("-- migrate:up\nCREATE TABLE foo (id INTEGER);\n")
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
	hlf := ComputeChecksum(lf)
	hcrlf := ComputeChecksum(crlf)
	if hlf != hcrlf {
		t.Fatalf("checksums differ for CRLF vs LF: %s != %s", hlf, hcrlf)
	}
	// BOM  CRLF
	withBOM := append(bom, crlf...)
	h3 := ComputeChecksum(withBOM)
	if h3 != hlf {
		t.Fatalf("checksums differ for BOMCRLF vs LF: %s != %s", h3, hlf)
	}
}
