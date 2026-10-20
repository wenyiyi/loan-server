//go:build integration

package integration

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Compare the checked-in API paths with the actual production router.
func TestOpenAPIRoutesMatchServer(t *testing.T) {
	f := newAPIFixture(t)
	data, err := os.ReadFile("../../api/openapi.yaml")
	require.NoError(t, err)
	var spec struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(data, &spec))
	require.Equal(t, "3.0.3", spec.OpenAPI)
	actual := map[string]bool{}
	for _, route := range f.router.(*gin.Engine).Routes() {
		actual[route.Method+" "+regexp.MustCompile(`:([a-z_]+)`).ReplaceAllString(route.Path, `{$1}`)] = true
	}
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	t.Logf("[Contract] Server routes=%v; API routes=%v", actual, documented)
	require.Equal(t, actual, documented, "OpenAPI must describe every route, with no extra routes")
}
