package od

// This file regroups OD extensions that are executed when reading or writing to object dictionary

import (
	"io"
	"os"

	log "github.com/sirupsen/logrus"
)

type FileObject struct {
	FilePath  string
	WriteMode int
	ReadMode  int
	File      *os.File
}

// [SDO] Custom function for reading a file like object
func ReadEntryFileObject(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || stream.Subindex != 0 || stream.Object == nil {
		return ODR_DEV_INCOMPAT
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return ODR_DEV_INCOMPAT
	}
	if stream.DataOffset == 0 {
		var err error
		log.Infof("[OD][EXTENSION][FILE] opening %v for reading", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.ReadMode, 0644)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
	}
	countReadInt, err := io.ReadFull(fileObject.File, data)

	switch err {
	case nil:
		*countRead = uint16(countReadInt)
		stream.DataOffset += uint32(countReadInt)
		return ODR_PARTIAL
	case io.EOF, io.ErrUnexpectedEOF:
		*countRead = uint16(countReadInt)
		log.Infof("[OD][EXTENSION][FILE] finished reading %v", fileObject.FilePath)
		fileObject.File.Close()
		return nil
	default:
		// unexpected error
		log.Errorf("[OD][EXTENSION][FILE] error reading file %v", err)
		fileObject.File.Close()
		return ODR_DEV_INCOMPAT

	}
}

// [SDO] Custom function for writing a file like object
func WriteEntryFileObject(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex != 0 || stream.Object == nil {
		return ODR_DEV_INCOMPAT
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return ODR_DEV_INCOMPAT
	}
	if stream.DataOffset == 0 {
		var err error
		log.Infof("[OD][EXTENSION][FILE] opening %v for writing", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.WriteMode, 0644)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
	}

	countWrittenInt, err := fileObject.File.Write(data)
	if err == nil {
		*countWritten = uint16(countWrittenInt)
		stream.DataOffset += uint32(countWrittenInt)
		if stream.DataLength == stream.DataOffset {
			log.Infof("[OD][EXTENSION][FILE] finished writing %v", fileObject.FilePath)
			fileObject.File.Close()
			return nil
		} else {
			return ODR_PARTIAL
		}
	} else {
		log.Errorf("[OD][EXTENSION][FILE] error writing file %v", err)
		fileObject.File.Close()
		return ODR_DEV_INCOMPAT
	}

}

// [SDO] Custom function for reading an io.Reader
func ReadEntryReader(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || stream.Subindex != 0 || stream.Object == nil {
		return ODR_DEV_INCOMPAT
	}
	reader, ok := stream.Object.(io.ReadSeeker)
	if !ok {
		stream.DataOffset = 0
		return ODR_DEV_INCOMPAT
	}
	// If first read, go back to initial point
	if stream.DataOffset == 0 {
		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			return ODR_DEV_INCOMPAT
		}
	}
	// Read len(data) bytes
	countReadInt, err := io.ReadFull(reader, data)
	switch err {
	case nil:
		// Not finished reading
		*countRead = uint16(countReadInt)
		stream.DataOffset += uint32(countReadInt)
		return ODR_PARTIAL
	case io.EOF, io.ErrUnexpectedEOF:
		*countRead = uint16(countReadInt)
		log.Infof("[OD][EXTENSION][FILE] finished reading")
		return nil
	default:
		log.Errorf("[OD][EXTENSION][FILE] error reading file %v", err)
		return ODR_DEV_INCOMPAT

	}
}
