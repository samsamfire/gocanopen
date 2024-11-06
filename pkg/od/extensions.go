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
		return ErrDevIncompat
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return ErrDevIncompat
	}
	if stream.DataOffset == 0 {
		var err error
		log.Infof("[OD][EXTENSION][FILE] opening %v for reading", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.ReadMode, 0644)
		if err != nil {
			return ErrDevIncompat
		}
	} else {
		// Re-adjust file cursor depending on datoffset
		offset, err := fileObject.File.Seek(0, io.SeekCurrent)
		if err == nil {
			log.Info("we are now at %v", offset)
		}
		offset, err = fileObject.File.Seek(int64(stream.DataOffset), 0)
		if err == nil {
			log.Info("we are now at %v", offset)
		}
		if err != nil {
			return ErrDevIncompat
		}
	}
	countReadInt, err := io.ReadFull(fileObject.File, data)

	switch err {
	case nil:
		*countRead = uint16(countReadInt)
		stream.DataOffset += uint32(countReadInt)
		return ErrPartial
	case io.EOF, io.ErrUnexpectedEOF:
		*countRead = uint16(countReadInt)
		log.Infof("[OD][EXTENSION][FILE] finished reading %v", fileObject.FilePath)
		fileObject.File.Close()
		return nil
	default:
		// unexpected error
		log.Errorf("[OD][EXTENSION][FILE] error reading file %v", err)
		fileObject.File.Close()
		return ErrDevIncompat

	}
}

// [SDO] Custom function for writing a file like object
func WriteEntryFileObject(stream *Stream, data []byte, countWritten *uint16) error {
	if stream == nil || data == nil || countWritten == nil || stream.Subindex != 0 || stream.Object == nil {
		return ErrDevIncompat
	}
	fileObject, ok := stream.Object.(*FileObject)
	if !ok {
		stream.DataOffset = 0
		return ErrDevIncompat
	}
	if stream.DataOffset == 0 {
		var err error
		log.Infof("[OD][EXTENSION][FILE] opening %v for writing", fileObject.FilePath)
		fileObject.File, err = os.OpenFile(fileObject.FilePath, fileObject.WriteMode, 0644)
		if err != nil {
			return ErrDevIncompat
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
			return ErrPartial
		}
	} else {
		log.Errorf("[OD][EXTENSION][FILE] error writing file %v", err)
		fileObject.File.Close()
		return ErrDevIncompat
	}

}

// [SDO] Custom function for reading an io.Reader
func ReadEntryReader(stream *Stream, data []byte, countRead *uint16) error {
	if stream == nil || data == nil || countRead == nil || stream.Subindex != 0 || stream.Object == nil {
		return ErrDevIncompat
	}
	reader, ok := stream.Object.(io.ReadSeeker)
	if !ok {
		stream.DataOffset = 0
		return ErrDevIncompat
	}
	// If first read, go back to initial point
	if stream.DataOffset == 0 {
		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			return ErrDevIncompat
		}
	}
	// Read len(data) bytes
	countReadInt, err := io.ReadFull(reader, data)
	switch err {
	case nil:
		// Not finished reading
		*countRead = uint16(countReadInt)
		stream.DataOffset += uint32(countReadInt)
		return ErrPartial
	case io.EOF, io.ErrUnexpectedEOF:
		*countRead = uint16(countReadInt)
		log.Infof("[OD][EXTENSION][FILE] finished reading")
		return nil
	default:
		log.Errorf("[OD][EXTENSION][FILE] error reading file %v", err)
		return ErrDevIncompat

	}
}
