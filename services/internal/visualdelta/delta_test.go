package visualdelta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
	"testing"
)

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Independent wire decoder verifies the encoder without reusing its matching.
func decode(t *testing.T, base, patch []byte) []byte {
	t.Helper()
	if len(patch) < 80 || string(patch[:8]) != "OCCDLT01" {
		t.Fatal("header")
	}
	n := int(binary.LittleEndian.Uint32(patch[72:76]))
	out := make([]byte, 0, n)
	count := int(binary.LittleEndian.Uint32(patch[76:80]))
	patch = patch[80:]
	for range count {
		op := patch[0]
		size := int(binary.LittleEndian.Uint32(patch[1:5]))
		patch = patch[5:]
		if op == 1 {
			out = append(out, patch[:size]...)
			patch = patch[size:]
			continue
		}
		offset := int(binary.LittleEndian.Uint32(patch[:4]))
		patch = patch[4:]
		start := len(out)
		out = append(out, base[offset:offset+size]...)
		if op == 2 {
			delta := binary.LittleEndian.Uint32(patch[:4])
			patch = patch[4:]
			for i := start; i < len(out); i += 4 {
				binary.LittleEndian.PutUint32(out[i:], binary.LittleEndian.Uint32(out[i:])+delta)
			}
		}
	}
	if len(out) != n || len(patch) != 0 {
		t.Fatal("size")
	}
	return out
}

func TestExactShiftedBinaryAndIndexTranslation(t *testing.T) {
	base := make([]byte, 1<<20)
	random := rand.New(rand.NewPCG(17, 29))
	for i := range base {
		base[i] = byte(random.Uint32())
	}
	for _, offset := range []int{0, 1, 4, 127} {
		target := append(bytes.Repeat([]byte{13}, offset), base...)
		for i := offset + 64<<10; i+4 < len(target); i += 4 {
			binary.LittleEndian.PutUint32(target[i:], binary.LittleEndian.Uint32(target[i:])+79)
		}
		patch, e := Encode(t.Context(), base, target, digest(base), digest(target))
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(decode(t, base, patch), target) {
			t.Fatal("not exact")
		}
		if offset%4 == 0 && len(patch) > len(target)/8 {
			t.Fatalf("lost shifted-buffer reuse: %d", len(patch))
		}
	}
}
func TestLiteralEmptyCollisionAndCancellation(t *testing.T) {
	for _, base := range [][]byte{nil, bytes.Repeat([]byte{0}, 16384), bytes.Repeat([]byte{1, 2, 3, 4}, 8192)} {
		for _, target := range [][]byte{nil, []byte{1, 2, 3}, bytes.Repeat([]byte{4, 3, 2, 1}, 8192)} {
			patch, e := Encode(t.Context(), base, target, digest(base), digest(target))
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(decode(t, base, patch), target) {
				t.Fatal("literal/collision")
			}
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := Encode(ctx, make([]byte, 8192), make([]byte, 8192), digest(nil), digest(nil)); e != context.Canceled {
		t.Fatalf("cancellation: %v", e)
	}
}
