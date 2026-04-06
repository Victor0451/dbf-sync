package dbf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// DBFRecord represents a single record from a DBF file
type DBFRecord map[string]interface{}

// DBFFile represents an open DBF file with metadata
type DBFFile struct {
	path           string
	file           *os.File
	header         *dbfHeader
	fields         []dbfField
	encoding       encoding.Encoding
	recordLength   int
	currentOffset  int64
	recordsRead    int
}

// dbfHeader represents the header of a DBF file
type dbfHeader struct {
	version       byte
	year          int
	month         int
	day           int
	recordCount   int64
	headerSize    int16
	recordLength  int16
	reserved1     [2]byte
	reserved2     [2]byte
	reserved3     [4]byte
}

// dbfField represents a field definition in a DBF file
type dbfField struct {
	name      string
	fieldType byte
	length    byte
	decimals  byte
}

// Common field type constants
const (
	FieldTypeCharacter = 'C'
	FieldTypeDate     = 'D'
	FieldTypeFloat    = 'F'
	FieldTypeNumeric  = 'N'
	FieldTypeLogical  = 'L'
	FieldTypeMemo     = 'M'
)

// DBFOpenOptions contains options for opening a DBF file
type DBFOpenOptions struct {
	// Encoding to use for character fields
	// If nil, will try to detect from LDID or use ISO-8859-1 (Latin1) as fallback
	Encoding encoding.Encoding

	// SkipDeleted records (default: true)
	SkipDeleted bool
}

// Default options
var defaultOpenOptions = &DBFOpenOptions{
	Encoding:    charmap.ISO8859_1,
	SkipDeleted:  true,
}

// OpenDBF opens a DBF file and returns a DBFFile handle
func OpenDBF(path string, opts ...DBFOpenOptions) (*DBFFile, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %v", ErrFileNotFound, err)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	options := defaultOpenOptions
	if len(opts) > 0 {
		options = &opts[0]
	}

	// If no encoding specified, try to detect from LDID or default to Latin1
	if options.Encoding == nil {
		options.Encoding = charmap.ISO8859_1
	}

	dbf := &DBFFile{
		path:     path,
		file:     file,
		encoding: options.Encoding,
	}

	// Read and parse header
	if err := dbf.readHeader(); err != nil {
		dbf.Close()
		return nil, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}

	dbf.recordLength = int(dbf.header.recordLength)
	dbf.currentOffset = int64(dbf.header.headerSize)

	return dbf, nil
}

// readHeader reads and parses the DBF file header
func (d *DBFFile) readHeader() error {
	headerBytes := make([]byte, 32)
	if _, err := io.ReadFull(d.file, headerBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}

	header := &dbfHeader{
		version:       headerBytes[0],
		year:          int(headerBytes[1]) + 1900,
		month:         int(headerBytes[2]),
		day:           int(headerBytes[3]),
		recordCount:    int64(bytesToUint32LE(headerBytes[4:8])),
		headerSize:    int16(bytesToUint16LE(headerBytes[8:10])),
		recordLength:  int16(bytesToUint16LE(headerBytes[10:12])),
	}

	// Validate version
	if header.version != 0x03 && header.version != 0x83 && header.version != 0x8B {
		return fmt.Errorf("%w: unsupported DBF version: 0x%02X (only dBase III supported)", ErrInvalidFormat, header.version)
	}

	d.header = header

	// Read field definitions
	fields := make([]dbfField, 0)
	fieldHeaderSize := int(header.headerSize) - 32
	fieldCount := fieldHeaderSize / 32

	for i := 0; i < fieldCount; i++ {
		fieldBytes := make([]byte, 32)
		if _, err := io.ReadFull(d.file, fieldBytes); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidFormat, err)
		}

		// Field name is null-terminated
		name := string(bytes.Trim(fieldBytes[0:11], "\x00"))
		if name == "" {
			break // End of field definitions
		}

		fields = append(fields, dbfField{
			name:      strings.TrimSpace(name),
			fieldType: fieldBytes[11],
			length:    fieldBytes[16],
			decimals:  fieldBytes[17],
		})
	}

	d.fields = fields

	// Skip to end of header (may include field descriptor array terminator 0x0D)
	headerTerminator := make([]byte, 1)
	if _, err := d.file.Read(headerTerminator); err != nil {
		return fmt.Errorf("failed to read header terminator: %w", err)
	}

	return nil
}

// FieldNames returns the list of field names in the DBF file
func (d *DBFFile) FieldNames() []string {
	names := make([]string, len(d.fields))
	for i, f := range d.fields {
		names[i] = f.name
	}
	return names
}

// RecordCount returns the number of records in the DBF file
func (d *DBFFile) RecordCount() int64 {
	return d.header.recordCount
}

// ReadAll reads all records from the DBF file
func (d *DBFFile) ReadAll() ([]DBFRecord, error) {
	records := make([]DBFRecord, 0, d.header.recordCount)

	for {
		record, err := d.ReadNext()
		if err != nil {
			return nil, err
		}
		if record == nil {
			break // End of file
		}
		records = append(records, record)
	}

	return records, nil
}

// ReadFiltered reads all records that match the filter function
func (d *DBFFile) ReadFiltered(filter func(DBFRecord) bool) ([]DBFRecord, error) {
	records := make([]DBFRecord, 0)

	for {
		record, err := d.ReadNext()
		if err != nil {
			return nil, err
		}
		if record == nil {
			break // End of file
		}

		if filter == nil || filter(record) {
			records = append(records, record)
		}
	}

	return records, nil
}

// ReadNext reads the next record from the DBF file
// Returns nil when end of file is reached
func (d *DBFFile) ReadNext() (DBFRecord, error) {
	for {
		// Check if we've read all records
		if d.recordsRead >= int(d.header.recordCount) {
			return nil, nil
		}

		// Read record
		recordBytes := make([]byte, d.recordLength)
		n, err := io.ReadFull(d.file, recordBytes)
		if err != nil {
			if err == io.EOF || n == 0 {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to read record: %w", err)
		}

		d.recordsRead++

		// Check if deleted — skip without recursing
		if recordBytes[0] == '*' {
			continue
		}

		// Parse fields
		record := make(DBFRecord)
		offset := 1 // Skip deletion flag

		for _, field := range d.fields {
			if offset >= len(recordBytes) {
				break
			}

			fieldData := recordBytes[offset : offset+int(field.length)]
			value := d.parseFieldValue(field, fieldData)
			record[field.name] = value

			offset += int(field.length)
		}

		return record, nil
	}
}

// parseFieldValue converts raw field data to appropriate Go type
func (d *DBFFile) parseFieldValue(field dbfField, data []byte) interface{} {
	str := string(bytes.TrimRight(data, " \x00"))

	switch field.fieldType {
	case FieldTypeCharacter:
		// Convert from configured encoding to UTF-8
		if d.encoding != nil {
			utf8Bytes, _, err := transform.Bytes(d.encoding.NewDecoder(), []byte(str))
			if err == nil {
				str = string(utf8Bytes)
			}
		}
		return strings.TrimSpace(str)

	case FieldTypeNumeric, FieldTypeFloat:
		str = strings.TrimSpace(str)
		if str == "" {
			return nil
		}
		// Try to parse as float first
		var f float64
		if _, err := fmt.Sscanf(str, "%f", &f); err == nil {
			if field.decimals == 0 {
				// Return as int if no decimals specified
				return int64(f)
			}
			return f
		}
		return nil

	case FieldTypeDate:
		if len(str) != 8 {
			return nil
		}
		// Format: YYYYMMDD
		var year, month, day int
		if _, err := fmt.Sscanf(str, "%4d%2d%2d", &year, &month, &day); err != nil {
			return nil
		}
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	case FieldTypeLogical:
		str = strings.ToUpper(str)
		switch str {
		case "Y", "T":
			return true
		case "N", "F":
			return false
		default:
			return nil
		}

	case FieldTypeMemo:
		return str // Memo fields contain pointer to FPT file
	}

	return str
}

// Close closes the DBF file
func (d *DBFFile) Close() error {
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}

// Helper functions for reading little-endian values
func bytesToUint16LE(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

func bytesToUint32LE(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// Custom errors
var (
	ErrInvalidFormat = errors.New("invalid DBF file format")
	ErrFileNotFound  = errors.New("DBF file not found")
)