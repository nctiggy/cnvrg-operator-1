package registry

import (
	"github.com/cnvrg-operator/pkg/desired"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const path = "/pkg/registry/tmpl"

func registryState() []*desired.State {
	return []*desired.State{
		{
			TemplatePath:   path + "/secret.tpl",
			Template:       nil,
			ParsedTemplate: "",
			Obj:            &unstructured.Unstructured{},
			GVR:            desired.Kinds[desired.SecretGVR],
			Own:            true,
		},
	}
}

func State(data interface{}) []*desired.State {
	registry := registryState()
	registry[0].TemplateData = data
	return registry
}
