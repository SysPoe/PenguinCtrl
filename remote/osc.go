package remote

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/syspoe/cusus/internal/osc"
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
			encoded, err := osc.AppendString(nil, value.Value)
			if err != nil {
				return nil, err
			}
			args = append(args, encoded)
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
	payload, err := osc.AppendString(nil, address)
	if err != nil {
		return nil, err
	}
	payload, err = osc.AppendString(payload, tags.String())
	if err != nil {
		return nil, err
	}
	for _, arg := range args {
		payload = append(payload, arg...)
	}
	return payload, nil
}
