package command

import (
	"encoding/json"
	"strings"
)

// Arguments is the JSON object the -args flag holds, as the map a run is handed.
//
// Each number stays the text it was written in (a json.Number), so the run can tell
// an int from a float and keep an int exact whatever its size: a number read to a
// float64 would round, and one past its range would be refused with the whole number
// in the words (Q4, Q437). Text that is not JSON gets the words json.Unmarshal has
// always given it. JSON that is not an object is an error too.
//
// Revisions:
//   - 2026-11-13 13:12: initial creation, out of _Supplying (C132, Q4, Q437)
func Arguments(supplied string) (map[string]any, error) {
	var (
		args map[string]any
		text = []byte(supplied)
	)

	if !json.Valid(text) {
		return nil, json.Unmarshal(
			text,
			&args,
		)
	}

	reading := json.NewDecoder(strings.NewReader(supplied))

	reading.UseNumber()

	err := reading.Decode(&args)
	if err != nil {
		return nil, err
	}

	return args, nil
}
