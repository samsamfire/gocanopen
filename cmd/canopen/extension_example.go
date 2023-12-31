package main

import (
	"io"
	"os"

	canopen "github.com/samsamfire/gocanopen"
	log "github.com/sirupsen/logrus"
)

type DomainObjectExample struct {
	File   *os.File
	Reader *io.Reader
	Writer *io.Writer
}

// Block transfer read to 0x200F, does not really do anything
func ReadEntry200F(stream *canopen.Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || stream.Subindex != 0 {
		return canopen.ODR_DEV_INCOMPAT
	}
	if stream.DataOffset == 0 {
		// This is the first call so create DomainObjectExample variable for reading
		var err error
		domainObject := DomainObjectExample{File: nil, Reader: nil}
		domainObject.File, err = os.OpenFile("OD_file_domain.bin", os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return canopen.ODR_DEV_INCOMPAT
		}
		stream.Object = &domainObject
	}
	obj, ok := stream.Object.(*DomainObjectExample)
	if !ok {
		log.Error("invalid return type")
		stream.DataOffset = 0
		return canopen.ODR_DEV_INCOMPAT
	}
	countReadInt, err := io.ReadFull(obj.File, data)
	switch err {
	case nil:
		*countRead = uint16(countReadInt)
		log.Infof("Read %v", countReadInt)
		stream.DataOffset += uint32(countReadInt)
		return canopen.ODR_PARTIAL
	case io.EOF, io.ErrUnexpectedEOF:
		*countRead = uint16(countReadInt)
		log.Info("Reached end of file")
		obj.File.Close()
		return nil
	default:
		//unexpected error
		log.Errorf("error reading file %v", err)
		obj.File.Close()
		return canopen.ODR_DEV_INCOMPAT

	}
}

// Block transfer write to 0x200F, does not really do anything
func WriteEntry200F(stream *canopen.Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex != 0 {
		return canopen.ODR_DEV_INCOMPAT
	}

	if stream.DataOffset == 0 {
		// This is the first call so create DomainObjectExample variable for reading
		var err error
		domainObject := DomainObjectExample{File: nil, Reader: nil}
		domainObject.File, err = os.OpenFile("OD_file_domain.bin", os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return canopen.ODR_DEV_INCOMPAT
		}
		stream.Object = &domainObject
	}
	obj, ok := stream.Object.(*DomainObjectExample)
	if !ok {
		log.Error("invalid return type")
		stream.DataOffset = 0
		return canopen.ODR_DEV_INCOMPAT
	}
	countWrittenInt, err := obj.File.Write(data)
	if err == nil {
		*countWritten = uint16(countWrittenInt)
		stream.DataOffset += uint32(countWrittenInt)
		if stream.DataLength == stream.DataOffset {
			log.Infof("Reached the end of the download")
			return nil
		} else {
			return canopen.ODR_PARTIAL
		}
	} else {
		//unexpected error
		log.Errorf("error reading file %v", err)
		obj.File.Close()
		return canopen.ODR_DEV_INCOMPAT
	}

}
