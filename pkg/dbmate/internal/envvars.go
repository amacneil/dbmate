package internal

import (
	"bytes"
	"os"
	"strings"
	"text/template"
)

func GetEnvMap() map[string]string {
	envMap := make(map[string]string)

	for _, envVar := range os.Environ() {
		entry := strings.SplitN(envVar, "=", 2)
		envMap[entry[0]] = entry[1]
	}

	return envMap
}

func ResolveRefs(snippet string, envVars []string, envMap map[string]string) (string, error) {
	if envVars == nil {
		return snippet, nil
	}

	model := make(map[string]string, len(envVars))
	for _, envVar := range envVars {
		model[envVar] = envMap[envVar]
	}

	template := template.Must(template.New("tmpl").Option("missingkey=error").Parse(snippet))

	var buffer bytes.Buffer
	if err := template.Execute(&buffer, model); err != nil {
		return "", err
	}

	return buffer.String(), nil
}
