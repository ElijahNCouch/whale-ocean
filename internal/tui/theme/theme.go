package theme

import "github.com/charmbracelet/lipgloss"

// Palette is the single built-in whale TUI chrome palette.
// It centralizes the current color choices without introducing user-facing
// theme configuration yet.
type Palette struct {
	Text       lipgloss.Color
	Background lipgloss.Color
	Accent     lipgloss.Color
	Assistant  lipgloss.Color
	Border     lipgloss.Color
	Muted      lipgloss.Color
	Subtle     lipgloss.Color
	Info       lipgloss.Color
	InfoSoft   lipgloss.Color
	Success    lipgloss.Color
	Warn       lipgloss.Color
	Error      lipgloss.Color
	Palette    lipgloss.Color
	StatusIdle lipgloss.Color
	Selection  lipgloss.Color

	UserAccent     lipgloss.Color
	UserBackground lipgloss.Color

	Plan           lipgloss.Color
	PlanBackground lipgloss.Color
	Tool           lipgloss.Color
	Result         lipgloss.Color
	ResultDenied   lipgloss.Color
	ResultTimeout  lipgloss.Color
	ResultError    lipgloss.Color
	ResultRunning  lipgloss.Color
}

// Deep-water ramp, darkest first. Everything structural — the frame fill, the
// raised blocks a prompt or plan sits on, borders and dimmed text — comes from
// this ladder, so the chrome reads as one body of water rather than as grey
// boxes that happen to sit on a blue field.
//
// The base is pushed dark and saturated on purpose: these are the colours the
// signal palette below is seen against, and a neon cyan only reads as neon
// when the water behind it is deep.
const (
	abyss   = "#041427" // frame fill
	trench  = "#0a2a44" // raised block (plan)
	shelf   = "#0b2f4d" // raised block (user prompt)
	current = "#10496b" // selection wash
	reef    = "#1f6b9e" // borders
	shallow = "#2f7fa8" // subtle dividers
	foam    = "#8fd0f0" // dimmed text
	spray   = "#e6f7ff" // body text
)

// Signal colours, run at full saturation. These are lit rather than tinted:
// bioluminescence against deep water, not pastels on navy.
//
// Two of them break out of the blues on purpose. A warning takes a sandbar
// amber and an error a hot coral, because an alarm that shares the hue of the
// chrome is an alarm nobody sees. Everything else stays in the water.
const (
	electric = "#00d9ff" // brand, user accent
	glacier  = "#3df0ff" // assistant
	iceBlue  = "#7af0ff"
	openBlue = "#2b8bff"
	aqua     = "#00e5c0" // tools
	seafoam  = "#00f5a0" // success
	anemone  = "#b06cff" // shell operators, search
	sandbar  = "#ffae00" // warnings
	amber    = "#ffd23f" // timeouts
	coral    = "#ff3d6e" // errors
	deepRed  = "#ff1f4f" // hard failures
	current2 = "#59c9ff" // running
)

var Default = Palette{
	Text:           lipgloss.Color(spray),
	Background:     lipgloss.Color(abyss),
	Accent:         lipgloss.Color(electric),
	Assistant:      lipgloss.Color(glacier),
	Border:         lipgloss.Color(reef),
	Muted:          lipgloss.Color(foam),
	Subtle:         lipgloss.Color(shallow),
	Info:           lipgloss.Color(openBlue),
	InfoSoft:       lipgloss.Color(iceBlue),
	Success:        lipgloss.Color(seafoam),
	Warn:           lipgloss.Color(sandbar),
	Error:          lipgloss.Color(coral),
	Palette:        lipgloss.Color(anemone),
	StatusIdle:     lipgloss.Color(aqua),
	Selection:      lipgloss.Color(current),
	UserAccent:     lipgloss.Color(electric),
	UserBackground: lipgloss.Color(shelf),
	Plan:           lipgloss.Color(openBlue),
	PlanBackground: lipgloss.Color(trench),
	Tool:           lipgloss.Color(aqua),
	Result:         lipgloss.Color(glacier),
	ResultDenied:   lipgloss.Color(sandbar),
	ResultTimeout:  lipgloss.Color(amber),
	ResultError:    lipgloss.Color(deepRed),
	ResultRunning:  lipgloss.Color(current2),
}

func UserPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(Default.UserBackground)
}

func UserPromptGlyphStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Default.UserAccent).Bold(true)
}

func MutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Default.Muted)
}

func StatusStyle(kind string) lipgloss.Style {
	switch kind {
	case "success":
		return lipgloss.NewStyle().Foreground(Default.Success)
	case "warning", "warn":
		return lipgloss.NewStyle().Foreground(Default.Warn)
	case "error":
		return lipgloss.NewStyle().Foreground(Default.Error)
	default:
		return MutedStyle()
	}
}

func RoleBorder(role string) lipgloss.Color {
	switch role {
	case "you":
		return Default.Accent
	case "assistant":
		return Default.Assistant
	case "think":
		return Default.Border
	case "notice", "info", "result_canceled", "result_neutral", "shell_result_neutral":
		return Default.Muted
	case "status":
		return Default.Info
	case "plan":
		return Default.Plan
	case "tool":
		return Default.Tool
	case "result":
		return Default.Result
	case "result_ok", "shell_result_ok":
		return Default.Success
	case "result_nonzero", "shell_result_nonzero":
		return Default.Warn
	case "result_denied", "shell_result_denied":
		return Default.ResultDenied
	case "result_failed", "shell_result_failed", "error":
		return Default.Error
	case "result_blocked", "shell_result_blocked", "result_mode_hint", "shell_result_mode_hint", "result_http_error", "shell_result_http_error", "result_usage_hint", "shell_result_usage_hint", "result_recoverable":
		return Default.Warn
	case "result_timeout", "shell_result_timeout":
		return Default.ResultTimeout
	case "result_error", "shell_result_error":
		return Default.ResultError
	case "result_running", "shell_result_running":
		return Default.ResultRunning
	case "shell_result_canceled":
		return Default.Muted
	default:
		return Default.Border
	}
}
