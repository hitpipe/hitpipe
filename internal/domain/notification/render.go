package notification

import (
	"bytes"
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

func RenderTemplate(templateName string, data interface{}) (string, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/"+templateName)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
