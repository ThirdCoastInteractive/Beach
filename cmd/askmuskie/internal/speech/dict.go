package speech

import (
	"bytes"
	_ "embed"
)

// cmudictData is the CMU Pronouncing Dictionary (cmusphinx/cmudict), embedded
// so the binary needs no data files at runtime. License: cmudict.LICENSE.
//
//go:embed cmudict.dict
var cmudictData []byte

// Default builds a G2P backed by the embedded CMU Pronouncing Dictionary.
func Default() (*G2P, error) {
	return New(bytes.NewReader(cmudictData))
}
