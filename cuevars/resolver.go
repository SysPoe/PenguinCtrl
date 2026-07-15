// Package cuevars resolves show cue templates against machine settings and the
// current cue number. It owns cue-number offset semantics outside config storage.
package cuevars

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/syspoe/cusus/config"
)

const maximumTemplateExpansions = 8

var templatePattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_.-]*)([+-]\d+(?:\.\d{1,2})?)?\}`)

func Resolve(template string, settings config.Settings, cueNumber string) string {
	values := make(map[string]string, len(settings.Variables)+3)
	for key, value := range settings.Variables {
		values[key] = value
	}
	values["defaultPlayback"] = settings.DefaultPlayback
	values["defaultMediaOutput"] = settings.DefaultMediaOutput
	values["cueNumber"] = cueNumber

	resolved := template
	for range maximumTemplateExpansions {
		next := templatePattern.ReplaceAllStringFunc(resolved, func(match string) string {
			parts := templatePattern.FindStringSubmatch(match)
			value, ok := values[parts[1]]
			if !ok {
				return match
			}
			if parts[1] == "cueNumber" && parts[2] != "" {
				if offsetValue, err := offsetCueNumber(value, parts[2]); err == nil {
					return offsetValue
				}
			}
			return value
		})
		if next == resolved {
			break
		}
		resolved = next
	}
	return resolved
}

func offsetCueNumber(base, offset string) (string, error) {
	baseValue, err := cueNumberToHundredths(base)
	if err != nil {
		return "", err
	}
	offsetValue, err := signedCueNumberToHundredths(offset)
	if err != nil {
		return "", err
	}
	value := max(0, baseValue+offsetValue)
	whole, fraction := value/100, value%100
	if fraction == 0 {
		return strconv.Itoa(whole), nil
	}
	return strings.TrimRight(fmt.Sprintf("%d.%02d", whole, fraction), "0"), nil
}

func cueNumberToHundredths(value string) (int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) > 2 || len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("invalid cue number %q", value)
	}
	whole, err := strconv.Atoi(parts[0])
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("invalid cue number %q", value)
	}
	fraction := 0
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 2 {
			return 0, fmt.Errorf("invalid cue number %q", value)
		}
		fraction, err = strconv.Atoi(parts[1] + strings.Repeat("0", 2-len(parts[1])))
		if err != nil {
			return 0, fmt.Errorf("invalid cue number %q", value)
		}
	}
	return whole*100 + fraction, nil
}

func signedCueNumberToHundredths(value string) (int, error) {
	if len(value) < 2 || (value[0] != '+' && value[0] != '-') {
		return 0, fmt.Errorf("invalid cue offset %q", value)
	}
	result, err := cueNumberToHundredths(value[1:])
	if err != nil {
		return 0, err
	}
	if value[0] == '-' {
		result = -result
	}
	return result, nil
}
