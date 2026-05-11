// templates.go
package pkg

import (
	"fmt"
	"strings"

	"github.com/manifoldco/promptui"
)

var PodTemplate = &promptui.SelectTemplates{
	Active:   fmt.Sprintf("Namespace: {{ .Namespace | blue }} | Pod: %s {{ .Name | cyan }}", promptui.IconSelect),
	Inactive: "Namespace: {{ .Namespace | blue }} | Pod: {{ .Name | magenta }}",
	Selected: fmt.Sprintf("Namespace: {{ .Namespace | blue }} | Pod: %s {{ .Name | cyan }}", promptui.IconGood),
}

var ContainerTemplate = &promptui.SelectTemplates{
	Active:   fmt.Sprintf("%s Container: {{ . | cyan | bold }}", promptui.IconSelect),
	Inactive: "  Container: {{ . | magenta}}",
	Selected: fmt.Sprintf("%s Container: {{ . | cyan }}", promptui.IconGood),
}

var IngressTemplate = &promptui.SelectTemplates{
	Active:   "\U0001F449 {{ .Name | cyan }} ({{ .Namespace | red }})",
	Inactive: "  {{ .Name | cyan }} ({{ .Namespace | red }})",
	Selected: "✅ Selected Ingress: {{ .Name | cyan }} ({{ .Namespace | red }})",
}

var ServiceTemplate = &promptui.SelectTemplates{
	Active:   "\U0001F449 {{ . | yellow }}",
	Inactive: "  {{ . | cyan }}",
	Selected: "✅ Selected Service: {{ . | yellow }}",
}

var PodTemplateIngress = &promptui.SelectTemplates{
	Active:   "\U0001F449 {{ .Name | cyan }} ({{ .IP | yellow }}, Node: {{ .NodeName | red }})",
	Inactive: "  {{ .Name | cyan }}",
	Selected: "✅ Selected Pod: {{ .Name | cyan }}",
	// 👇 Details satırı tamamen kaldırılıyor
}

func BuildDashedLine(header string) string {
	var out strings.Builder
	for _, ch := range header {
		if ch == '\t' {
			out.WriteRune('\t')
		} else {
			out.WriteRune('-')
		}
	}
	return out.String()
}
