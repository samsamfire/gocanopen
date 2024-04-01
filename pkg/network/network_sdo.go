package network

import (
	"bytes"
	"fmt"
	"io"

	"github.com/samsamfire/gocanopen/pkg/od"
)

// EDSFormatHandler takes a formatType and a reader
// to handle an EDS file stored as a proprietary format (zip, etc)
type EDSFormatHandler func(formatType uint8, reader io.Reader) (*od.ObjectDictionary, error)

// Read object dictionary using object 1021 (EDS storage) of a remote node
// Optional callback can be provided to perform manufacturer specific parsing
// in case a custom foramt is used
func (network *Network) ReadEDS(nodeId uint8, edsFormatHandler EDSFormatHandler) (*od.ObjectDictionary, error) {
	rawEds, err := network.ReadAll(nodeId, 0x1021, 0)
	if err != nil {
		return nil, err
	}
	edsFormat := []byte{0}
	_, err = network.ReadRaw(nodeId, 0x1022, 0, edsFormat)
	switch edsFormatHandler {
	case nil:
		// No callback & format is not specified or
		// Storage format is 0
		// Use default ASCII format
		if err != nil || edsFormat[0] == 0 {
			od, err := od.Parse(rawEds, nodeId)
			if err != nil {
				return nil, err
			}
			return od, nil
		} else {
			return nil, fmt.Errorf("supply a handler for the format : %v", edsFormat[0])
		}
	default:
		odReader := bytes.NewBuffer(rawEds)
		od, err := edsFormatHandler(edsFormat[0], odReader)
		if err != nil {
			return nil, err
		}
		return od, nil
	}
}
