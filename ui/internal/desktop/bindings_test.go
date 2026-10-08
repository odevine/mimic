package desktop

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The page reaches the services by name, so a method renamed on the Go side
// would only fail when someone clicked the button. These tests read the names
// out of api.js and the launch check page and hold them against the bound types

var boundTypes = map[string]reflect.Type{
	"Cards":     reflect.TypeFor[*Cards](),
	"Render":    reflect.TypeFor[*Render](),
	"List":      reflect.TypeFor[*List](),
	"Run":       reflect.TypeFor[*Run](),
	"Templates": reflect.TypeFor[*Templates](),
	"Settings":  reflect.TypeFor[*Settings](),
	"Overrides": reflect.TypeFor[*Overrides](),
	"Data":      reflect.TypeFor[*Data](),
	"Updates":   reflect.TypeFor[*Updates](),
	"System":    reflect.TypeFor[*System](),
}

// hooks are the lifecycle methods Wails calls itself, which the page never does
var hooks = []string{"ServiceStartup", "ServiceShutdown"}

// pageCalls returns the service and method pairs the frontend calls, as
// "Service.Method"
func pageCalls(t *testing.T) []string {
	t.Helper()
	var calls []string
	read := func(path string, re *regexp.Regexp, service string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			if service != "" {
				calls = append(calls, service+"."+m[1])
			} else {
				calls = append(calls, m[1]+"."+m[2])
			}
		}
	}
	read("../../frontend/js/api.js", regexp.MustCompile(`rpc\("(\w+)", "(\w+)"`), "")
	read("../../frontend/js/api.js", regexp.MustCompile(`rpc\(SYSTEM, "(\w+)"`), "System")
	read("smoke.js", regexp.MustCompile(`internal/desktop\.(\w+)\.(\w+)`), "")
	return calls
}

func TestPageCallsExistOnTheBoundTypes(t *testing.T) {
	calls := pageCalls(t)
	if len(calls) < 30 {
		t.Fatalf("found only %d page calls, so the patterns have drifted from api.js", len(calls))
	}
	for _, c := range calls {
		service, method, _ := strings.Cut(c, ".")
		typ, ok := boundTypes[service]
		if !ok {
			t.Errorf("api.js calls the unknown service %s", service)
			continue
		}
		if _, ok := typ.MethodByName(method); !ok {
			t.Errorf("api.js calls %s, which %s does not have", c, typ)
		}
	}
}

func TestBoundTypesExposeOnlyWhatThePageCalls(t *testing.T) {
	called := pageCalls(t)
	for service, typ := range boundTypes {
		for i := range typ.NumMethod() {
			name := typ.Method(i).Name
			if slices.Contains(hooks, name) || slices.Contains(called, service+"."+name) {
				continue
			}
			t.Errorf("%s.%s is bound but nothing in the page calls it", service, name)
		}
	}
}
