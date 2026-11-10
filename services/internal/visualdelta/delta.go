// Package visualdelta encodes exact immutable binary display artifacts. It has
// no topology/Revision semantics: all copies are verified against source bytes.
package visualdelta

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const ContentType = "application/vnd.occccad.visual-delta"
const MaxBytes = 64 << 20
const block = 4096
const header = 80
const polynomial uint64 = 0x9e3779b185ebca87

func word(b []byte, i int) uint32       { return binary.LittleEndian.Uint32(b[i : i+4]) }
func difference(b []byte, i int) uint64 { return uint64(word(b, i+4) - word(b, i)) }
func wordHash(b []byte, i int) uint64 {
	var h uint64
	for end := i + block - 4; i < end; i += 4 {
		h = h*polynomial + difference(b, i)
	}
	return h
}
func weak(b []byte, i int) (a, sum uint32) {
	for j := 0; j < block; j++ {
		a += uint32(b[i+j])
		sum += uint32(block-j) * uint32(b[i+j])
	}
	return
}
func weakKey(a, b uint32) uint32 { return a&65535 | b<<16 }

// Encode uses rolling byte matches and constant uint32 translations. The latter
// captures shifted GLB indices without interpreting them as persistent IDs.
// Hashes only select candidates; byte/word equality establishes every copy.
func Encode(ctx context.Context, base, target []byte, baseDigest, targetDigest string) ([]byte, error) {
	if len(base) > MaxBytes || len(target) > MaxBytes {
		return nil, fmt.Errorf("visual delta exceeds size budget")
	}
	baseSHA, e := hex.DecodeString(baseDigest)
	if e != nil || len(baseSHA) != 32 {
		return nil, fmt.Errorf("invalid base digest")
	}
	targetSHA, e := hex.DecodeString(targetDigest)
	if e != nil || len(targetSHA) != 32 {
		return nil, fmt.Errorf("invalid target digest")
	}
	patch := make([]byte, header)
	copy(patch, "OCCDLT01")
	copy(patch[8:40], baseSHA)
	copy(patch[40:72], targetSHA)
	binary.LittleEndian.PutUint32(patch[72:76], uint32(len(target)))
	put := func(v uint32) { patch = binary.LittleEndian.AppendUint32(patch, v) }
	var operations uint32
	literal := func(data []byte) {
		if len(data) == 0 {
			return
		}
		patch = append(patch, 1)
		put(uint32(len(data)))
		patch = append(patch, data...)
		operations++
	}
	copyRange := func(offset, length int, delta uint32) {
		if delta == 0 {
			patch = append(patch, 0)
		} else {
			patch = append(patch, 2)
		}
		put(uint32(length))
		put(uint32(offset))
		if delta != 0 {
			put(delta)
		}
		operations++
	}
	byteIndex := make(map[uint32][]int)
	wordIndex := make(map[uint64][]int)
	for i := 0; i+block <= len(base); i += block {
		if i%(256<<10) == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		a, b := weak(base, i)
		key := weakKey(a, b)
		if len(byteIndex[key]) < 8 {
			byteIndex[key] = append(byteIndex[key], i)
		}
		key64 := wordHash(base, i)
		if len(wordIndex[key64]) < 8 {
			wordIndex[key64] = append(wordIndex[key64], i)
		}
	}
	var power uint64 = 1
	for j := 0; j < block/4-2; j++ {
		power *= polynomial
	}
	i, start := 0, 0
	comparisonBudget := len(target)*16 + block*8
	var a, b uint32
	if len(target) >= block {
		a, b = weak(target, 0)
	}
	uStart := -block
	var uHash uint64
	for i+block <= len(target) {
		if i%(256<<10) == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		source, length, delta := -1, 0, uint32(0)
		for _, offset := range byteIndex[weakKey(a, b)] {
			comparisonBudget -= block
			if comparisonBudget < 0 {
				return nil, fmt.Errorf("visual delta comparison budget exhausted")
			}
			if bytes.Equal(base[offset:offset+block], target[i:i+block]) {
				source = offset
				length = block
				for source+length < len(base) && i+length < len(target) && base[source+length] == target[i+length] {
					length++
				}
				break
			}
		}
		if source < 0 && i%4 == 0 {
			if uStart == i-4 {
				uHash = (uHash-difference(target, i-4)*power)*polynomial + difference(target, i+block-8)
			} else {
				uHash = wordHash(target, i)
			}
			uStart = i
			for _, offset := range wordIndex[uHash] {
				comparisonBudget -= block
				if comparisonBudget < 0 {
					return nil, fmt.Errorf("visual delta comparison budget exhausted")
				}
				change := word(target, i) - word(base, offset)
				same := true
				for j := 4; j < block; j += 4 {
					if word(target, i+j)-word(base, offset+j) != change {
						same = false
						break
					}
				}
				if !same {
					continue
				}
				source = offset
				length = block
				delta = change
				for source+length+4 <= len(base) && i+length+4 <= len(target) && word(target, i+length)-word(base, source+length) == delta {
					length += 4
				}
				break
			}
		}
		if source >= 0 {
			literal(target[start:i])
			copyRange(source, length, delta)
			i += length
			start = i
			if i+block <= len(target) {
				a, b = weak(target, i)
			}
			continue
		}
		if i+block < len(target) {
			a = a - uint32(target[i]) + uint32(target[i+block])
			b = b - uint32(block)*uint32(target[i]) + a
		}
		i++
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	literal(target[start:])
	binary.LittleEndian.PutUint32(patch[76:80], operations)
	return patch, nil
}
