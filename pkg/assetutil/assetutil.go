package assetutil

import (
	"bytes"
	"text/template"
)

func Template(name, s string, data interface{}) (string, error) {
	t, err := template.New(name).Parse(s)
	if err != nil {
		return "", err
	}

	var b bytes.Buffer
	err = t.Execute(&b, data)
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

func MustTemplate(name, s string, data interface{}) string {
	res, err := Template(name, s, data)
	if err != nil {
		panic(err)
	}

	return res
}
