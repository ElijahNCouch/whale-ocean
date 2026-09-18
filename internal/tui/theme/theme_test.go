package theme

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Roles are asserted against palette fields rather than literal colours: the
// bug worth catching is a role wired to the wrong meaning, not a change of
// shade, and pinning hex here would only restate the palette.
func TestRoleBorderUsesTheRightPaletteEntry(t *testing.T) {
	cases := map[string]lipgloss.Color{
		"you":            Default.Accent,
		"assistant":      Default.Assistant,
		"plan":           Default.Plan,
		"tool":           Default.Tool,
		"result_ok":      Default.Success,
		"result_failed":  Default.Error,
		"result_running": Default.ResultRunning,
		"result_timeout": Default.ResultTimeout,
		"error":          Default.Error,
		"status":         Default.Info,
		"unknown":        Default.Border,
	}

	for role, want := range cases {
		if got := RoleBorder(role); got != want {
			t.Fatalf("role %q: want %s, got %s", role, want, got)
		}
	}
}

func TestPaletteIsFullyPopulatedTrueColour(t *testing.T) {
	v := reflect.ValueOf(Default)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		value := v.Field(i).Interface().(lipgloss.Color)
		if _, err := luminance(string(value)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// The fill sits behind every other colour, so anything darker than it would be
// invisible against it.
func TestBackgroundIsTheDarkestColour(t *testing.T) {
	bg, err := luminance(string(Default.Background))
	if err != nil {
		t.Fatalf("background: %v", err)
	}
	v := reflect.ValueOf(Default)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if name == "Background" {
			continue
		}
		got, err := luminance(string(v.Field(i).Interface().(lipgloss.Color)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got < bg {
			t.Fatalf("%s is darker than the background fill (%.3f < %.3f)", name, got, bg)
		}
	}
}

// Warnings and errors have to break out of the blues to stay readable as
// alarms; everything else should stay in the water.
func TestSignalColoursLeaveTheBlueRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		color lipgloss.Color
	}{
		{"Warn", Default.Warn},
		{"Error", Default.Error},
		{"ResultError", Default.ResultError},
		{"ResultTimeout", Default.ResultTimeout},
	} {
		r, g, b, err := rgb(string(tc.color))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if b >= r {
			t.Fatalf("%s is still blue-dominant (r=%d b=%d) and will not read as an alarm", tc.name, r, b)
		}
		_ = g
	}
}

func TestChromeColoursStayOceanic(t *testing.T) {
	for _, tc := range []struct {
		name  string
		color lipgloss.Color
	}{
		{"Background", Default.Background},
		{"Border", Default.Border},
		{"Subtle", Default.Subtle},
		{"Muted", Default.Muted},
		{"Selection", Default.Selection},
		{"UserBackground", Default.UserBackground},
		{"PlanBackground", Default.PlanBackground},
		{"Accent", Default.Accent},
		{"Assistant", Default.Assistant},
	} {
		r, _, b, err := rgb(string(tc.color))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if b <= r {
			t.Fatalf("%s is not blue-dominant (r=%d b=%d); the chrome should read as water", tc.name, r, b)
		}
	}
}

func TestSemanticStyles(t *testing.T) {
	if got := UserPromptStyle().GetBackground(); got != Default.UserBackground {
		t.Fatalf("user prompt background: want %s, got %s", Default.UserBackground, got)
	}
	if got := UserPromptGlyphStyle().GetForeground(); got != Default.UserAccent {
		t.Fatalf("user prompt glyph: want %s, got %s", Default.UserAccent, got)
	}
	if got := StatusStyle("success").GetForeground(); got != Default.Success {
		t.Fatalf("success status: want %s, got %s", Default.Success, got)
	}
}

func rgb(hex string) (int, int, int, error) {
	s := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("want a #rrggbb colour, got %q", hex)
	}
	out := make([]int, 3)
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseInt(s[i*2:i*2+2], 16, 0)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("want a #rrggbb colour, got %q", hex)
		}
		out[i] = int(v)
	}
	return out[0], out[1], out[2], nil
}

func luminance(hex string) (float64, error) {
	r, g, b, err := rgb(hex)
	if err != nil {
		return 0, err
	}
	return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 255.0, nil
}
