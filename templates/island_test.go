package templates

import (
	"bytes"
	"context"
	"github.com/robertguss/go-islands-starter/internal/dashboard"
	"strings"
	"testing"
)

func TestIslandEscapingAndRegistry(t *testing.T) {
	payload := `</script><img src=x onerror="alert(1)">`
	var out bytes.Buffer
	if err := Island("dashboard", dashboard.Props{Requests: []dashboard.Request{{Title: payload}}}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "<img") || strings.Contains(out.String(), "</script>") || !strings.Contains(out.String(), `\u003c`) {
		t.Fatal("unsafe props serialization")
	}
	out.Reset()
	if err := Island("unregistered", dashboard.Props{}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "data-island") || !strings.Contains(out.String(), `role="alert"`) {
		t.Fatal("unknown island did not fail closed")
	}
}
