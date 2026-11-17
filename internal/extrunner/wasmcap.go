// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package extrunner

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// PageSize is a WebAssembly memory page.
const PageSize = 64 * 1024

// capMemory returns the module with every memory it defines limited to
// maxPages. QuickJS's own memory limit does not catch every allocation
// (a growing array of arrays ran until the time limit in a probe), and a
// WebAssembly memory may grow to 4 GB unless the module declares a
// maximum, which wazero then enforces: malloc fails and the script gets
// "out of memory".
func capMemory(wasm []byte, maxPages uint64) ([]byte, error) {
	if len(wasm) < 8 || !bytes.Equal(wasm[:4], []byte("\x00asm")) {
		return nil, errors.New("extrunner: not a wasm module")
	}
	out := append([]byte{}, wasm[:8]...)
	found := false
	for i := 8; i < len(wasm); {
		id := wasm[i]
		size, n := binary.Uvarint(wasm[i+1:])
		if n <= 0 {
			return nil, errors.New("extrunner: bad section size")
		}
		start := i + 1 + n
		end := start + int(size)
		if end > len(wasm) {
			return nil, errors.New("extrunner: truncated section")
		}
		if id != 5 { // not the memory section
			out = append(out, wasm[i:end]...)
			i = end
			continue
		}
		body := wasm[start:end]
		count, k := binary.Uvarint(body)
		p := k
		nb := binary.AppendUvarint(nil, count)
		for j := uint64(0); j < count; j++ {
			if p >= len(body) {
				return nil, errors.New("extrunner: bad memory entry")
			}
			flag := body[p]
			p++
			min, m := binary.Uvarint(body[p:])
			p += m
			if flag&1 == 1 {
				_, m2 := binary.Uvarint(body[p:])
				p += m2
			}
			if min > maxPages {
				min = maxPages
			}
			nb = append(nb, flag|1)
			nb = binary.AppendUvarint(nb, min)
			nb = binary.AppendUvarint(nb, maxPages)
		}
		nb = append(nb, body[p:]...)
		out = append(out, 5)
		out = binary.AppendUvarint(out, uint64(len(nb)))
		out = append(out, nb...)
		found = true
		i = end
	}
	if !found {
		return nil, errors.New("extrunner: the module defines no memory")
	}
	return out, nil
}
