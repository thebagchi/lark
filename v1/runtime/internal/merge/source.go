package merge

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const (
	// SOURCE is the merged script, in one template. The program answers only
	// what each module's text is, cut into pieces; every decision about the
	// script is here: the order is the order given, and each module ends on a
	// line break, so the next module never starts inside a line.
	//
	// Revisions:
	//   - 2026-11-02 22:58: initial creation (C44a)
	SOURCE = `
{{- range $MODULE := . -}}
    {{- $LAST := "" -}}
    {{- range $PART := $MODULE -}}
        {{- $PART -}}
        {{- $LAST = $PART -}}
    {{- end -}}
    {{- if not (hasSuffix $LAST "\n") }}{{ "\n" }}{{ end -}}
{{- end -}}`
)

var (
	// PARSED is SOURCE, parsed once.
	//
	// Revisions:
	//   - 2026-11-03 06:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	PARSED = template.Must(
		template.New("source").Funcs(
			template.FuncMap{
				"hasSuffix": strings.HasSuffix,
			},
		).Parse(SOURCE),
	)
)

// _Rendered writes the modules' pieces as one script.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func _Rendered(modules [][]string) ([]byte, error) {
	var out bytes.Buffer

	err := PARSED.Execute(
		&out,
		modules,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"render the merged script: %w",
			err,
		)
	}

	return out.Bytes(), nil
}
