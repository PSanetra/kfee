package serdes

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// RawMessageType is the message type of records decoded without schema.
const RawMessageType = "(raw)"

// maxRawDepth limits the nesting of messages decoded without schema.
const maxRawDepth = 32

// DecodeRaw decodes protobuf data without schema, like protoc --decode_raw. Fields are keyed by
// their number. As the wire format does not tell the exact types, varints are shown as integers,
// fixed32/fixed64 as unsigned integers and length-delimited fields as nested message when they
// parse as one and are not printable text, otherwise as string, or as hex for binary data.
func DecodeRaw(data []byte) (DesData, error) {
	fields, err := parseRawMessage(data, 0)
	if err != nil {
		return DesData{}, fmt.Errorf("unable to decode record as protobuf: %w", err)
	}
	var buf bytes.Buffer
	writeRawMessage(&buf, fields)
	return DesData{Value: buf.String(), MessageType: RawMessageType}, nil
}

type rawField struct {
	number protowire.Number
	values []any
}

// rawMessage marks nested messages among the values of a field.
type rawMessage []rawField

func parseRawMessage(data []byte, depth int) (rawMessage, error) {
	if depth > maxRawDepth {
		return nil, errors.New("nested too deeply")
	}
	var fields rawMessage
	byNumber := map[protowire.Number]int{}
	for len(data) > 0 {
		number, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		if number < protowire.MinValidNumber || number > protowire.MaxValidNumber {
			return nil, fmt.Errorf("invalid field number %d", number)
		}
		data = data[n:]

		var value any
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			data = data[n:]
			value = varintValue(v)
		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			data = data[n:]
			value = v
		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			data = data[n:]
			value = v
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			data = data[n:]
			value = bytesValue(v, depth)
		case protowire.StartGroupType:
			v, n := protowire.ConsumeGroup(number, data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			data = data[n:]
			group, err := parseRawMessage(v, depth+1)
			if err != nil {
				return nil, err
			}
			value = group
		default:
			return nil, fmt.Errorf("invalid wire type %d of field %d", typ, number)
		}

		if i, ok := byNumber[number]; ok {
			fields[i].values = append(fields[i].values, value)
		} else {
			byNumber[number] = len(fields)
			fields = append(fields, rawField{number: number, values: []any{value}})
		}
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].number < fields[j].number })
	return fields, nil
}

// varintValue shows varints that only make sense as negative int64, like -1, as such.
func varintValue(v uint64) any {
	if v > math.MaxInt64 {
		return int64(v)
	}
	return v
}

func bytesValue(v []byte, depth int) any {
	if len(v) == 0 {
		return ""
	}
	if isPrintable(v) {
		return string(v)
	}
	if nested, err := parseRawMessage(v, depth+1); err == nil && len(nested) > 0 {
		return nested
	}
	if utf8.Valid(v) {
		return string(v)
	}
	return "0x" + hex.EncodeToString(v)
}

func isPrintable(v []byte) bool {
	if !utf8.Valid(v) {
		return false
	}
	for _, r := range string(v) {
		if !unicode.IsPrint(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func writeRawMessage(buf *bytes.Buffer, fields rawMessage) {
	buf.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Quote(strconv.Itoa(int(f.number))))
		buf.WriteByte(':')
		if len(f.values) == 1 {
			writeRawValue(buf, f.values[0])
			continue
		}
		buf.WriteByte('[')
		for j, v := range f.values {
			if j > 0 {
				buf.WriteByte(',')
			}
			writeRawValue(buf, v)
		}
		buf.WriteByte(']')
	}
	buf.WriteByte('}')
}

func writeRawValue(buf *bytes.Buffer, v any) {
	if nested, ok := v.(rawMessage); ok {
		writeRawMessage(buf, nested)
		return
	}
	data, _ := json.Marshal(v)
	buf.Write(data)
}
