package remote

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/syspoe/cusus/show"
)

func encodeOSC(address string, values []show.RemoteValue) ([]byte, error) {
	if !strings.HasPrefix(address, "/") || strings.ContainsRune(address, 0) {
		return nil, fmt.Errorf("invalid OSC address %q", address)
	}
	tags := strings.Builder{}
	tags.WriteByte(',')
	args := make([][]byte, 0, len(values))
	for _, value := range values {
		switch value.Type {
		case show.RemoteValueString:
			tags.WriteByte('s')
			args = append(args, oscString(value.Value))
		case show.RemoteValueInt:
			parsed, err := strconv.ParseInt(strings.TrimSpace(value.Value), 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid OSC integer %q", value.Value)
			}
			tags.WriteByte('i')
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, uint32(int32(parsed)))
			args = append(args, buf)
		case show.RemoteValueFloat:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value.Value), 32)
			if err != nil {
				return nil, fmt.Errorf("invalid OSC float %q", value.Value)
			}
			tags.WriteByte('f')
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, math.Float32bits(float32(parsed)))
			args = append(args, buf)
		case show.RemoteValueBool:
			parsed, err := strconv.ParseBool(strings.TrimSpace(value.Value))
			if err != nil {
				return nil, fmt.Errorf("invalid OSC boolean %q", value.Value)
			}
			if parsed {
				tags.WriteByte('T')
			} else {
				tags.WriteByte('F')
			}
		default:
			return nil, fmt.Errorf("unsupported OSC value type %d", value.Type)
		}
	}
	payload := append(oscString(address), oscString(tags.String())...)
	for _, arg := range args {
		payload = append(payload, arg...)
	}
	return payload, nil
}

func oscString(value string) []byte {
	raw := append([]byte(value), 0)
	// TODO(micro): name OSC 4-byte alignment pad as a constant (or shared OSC helper)
	for len(raw)%4 != 0 {
		raw = append(raw, 0)
	}
	return raw
}
