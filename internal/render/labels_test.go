package render

import (
	"slices"
	"testing"
)

func TestShortNames(t *testing.T) {
	cases := []struct{ in, want []string }{
		{[]string{"k8s/staging/configmap.yaml", "k8s/production/configmap.yaml"}, []string{"staging", "production"}},
		{[]string{"config/dev.env", "config/prod.env"}, []string{"dev.env", "prod.env"}},
		{[]string{"dev.env", "prod.env"}, []string{"dev.env", "prod.env"}},
		{[]string{"a/x.env", "a/b/x.env"}, []string{"x.env", "b/x.env"}},
		{[]string{"/abs/dev/.env", "/abs/prod/.env"}, []string{"dev", "prod"}},
		{[]string{"./a.env", "a.env"}, []string{"a.env", "a.env"}},
	}
	for _, c := range cases {
		if got := ShortNames(c.in); !slices.Equal(got, c.want) {
			t.Errorf("ShortNames(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
