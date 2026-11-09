package docfmt

import "time"

// ValidDate accepts a calendar day without a timezone or a time of day.
func ValidDate(value string) bool {
	if len(value) != 10 || value[:4] == "0000" {
		return false
	}
	day, err := time.Parse(time.DateOnly, value)
	return err == nil && day.Format(time.DateOnly) == value
}

func checkDate(obj map[string]any, path string) error {
	value, ok := obj["date"].(string)
	if !ok || !ValidDate(value) {
		return at(path+".date", "must be a valid calendar date in YYYY-MM-DD (0001 through 9999)")
	}
	for _, key := range []string{"text", "marks", "href", "user", "name", "dropdown"} {
		if _, has := obj[key]; has {
			return at(path+"."+key, "a date has only a date value")
		}
	}
	return nil
}
