package tools

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func newDevopsToolset(t *testing.T, root string) *Toolset {
	t.Helper()
	ts, err := NewToolset(root)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	return ts
}

// devopsPayload digs the tool's own result out of the standard envelope.
func devopsPayload(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected a data object, got %v", env)
	}
	payload, ok := data["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected a payload object, got %v", data)
	}
	return payload
}

func devopsToolByName(t *testing.T, ts *Toolset, name string) core.Tool {
	t.Helper()
	for _, tool := range ts.devopsTools() {
		if tool.Name() == name {
			return tool
		}
	}
	t.Skipf("tool %s is not offered on this machine", name)
	return nil
}

func runDevopsTool(t *testing.T, tool core.Tool, input string) map[string]any {
	t.Helper()
	res, err := tool.Run(context.Background(), core.ToolCall{ID: "1", Name: tool.Name(), Input: input})
	if err != nil {
		t.Fatalf("%s: %v", tool.Name(), err)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(res.ModelText), &env); err != nil {
		t.Fatalf("%s: result is not JSON: %v (%s)", tool.Name(), err, res.ModelText)
	}
	return env
}

// A tool nobody can run is worse than no tool: the model spends a turn
// discovering the binary is absent. The catalogue is gated on the binary
// existing, so this checks the gate rather than the command.
func TestDevopsToolsOnlyOfferedWhenBinaryExists(t *testing.T) {
	ts := newDevopsToolset(t, t.TempDir())
	offered := map[string]bool{}
	for _, tool := range ts.devopsTools() {
		offered[tool.Name()] = true
	}
	for _, spec := range devopsCommandSpecs() {
		_, err := exec.LookPath(spec.bin)
		hasBinary := err == nil
		if hasBinary != offered[spec.name] {
			t.Fatalf("%s: binary present=%v but tool offered=%v", spec.name, hasBinary, offered[spec.name])
		}
	}
}

// env_info needs no binary, and is the tool that tells a model what the rest
// of the catalogue is going to contain.
func TestEnvInfoAlwaysAvailableAndReportsTooling(t *testing.T) {
	ts := newDevopsToolset(t, t.TempDir())
	tool := devopsToolByName(t, ts, "env_info")
	env := runDevopsTool(t, tool, "{}")

	data := devopsPayload(t, env)
	if data["os"] == "" || data["arch"] == "" {
		t.Fatalf("expected os and arch, got %v", data)
	}
	available, _ := data["available_tools"].(map[string]any)
	missing, _ := data["missing_tools"].([]any)
	if len(available)+len(missing) != len(devopsCLIs) {
		t.Fatalf("every known CLI should be reported as present or missing: %d + %d != %d", len(available), len(missing), len(devopsCLIs))
	}
}

func TestGitStatusReportsBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "oceanic"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "user.name", "Test"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}

	ts := newDevopsToolset(t, dir)
	env := runDevopsTool(t, devopsToolByName(t, ts, "git_status"), "{}")
	data := devopsPayload(t, env)
	output, _ := data["output"].(string)
	if !strings.Contains(output, "oceanic") {
		t.Fatalf("expected the branch name in git status output, got %q", output)
	}
}

// Arguments are passed as argv, never through a shell, so a value containing
// shell punctuation has to stay one argument.
func TestDevopsArgumentsAreNotShellInterpreted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	sentinel := dir + "/pwned.txt"
	ts := newDevopsToolset(t, dir)
	tool := devopsToolByName(t, ts, "git_log")
	input, err := json.Marshal(map[string]any{"path": "x; touch " + sentinel})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	runDevopsTool(t, tool, string(input))

	if out, statErr := exec.Command("test", "-f", sentinel).CombinedOutput(); statErr == nil {
		t.Fatalf("argument was interpreted by a shell and created %s (%s)", sentinel, out)
	}
}

func TestPortCheckReportsClosedPort(t *testing.T) {
	ts := newDevopsToolset(t, t.TempDir())
	tool := devopsToolByName(t, ts, "port_check")
	// Port 1 on localhost is not something a developer machine serves.
	env := runDevopsTool(t, tool, `{"port":1,"timeout_ms":250}`)
	data := devopsPayload(t, env)
	if open, _ := data["open"].(bool); open {
		t.Fatalf("expected port 1 to be closed, got %v", data)
	}
	if msg, _ := data["error"].(string); msg == "" {
		t.Fatalf("expected a reason the port was not open, got %v", data)
	}
}

func TestPortCheckFindsAListeningPort(t *testing.T) {
	ln := mustListen(t)
	defer ln.Close()
	_, port := splitHostPort(t, ln.Addr().String())

	ts := newDevopsToolset(t, t.TempDir())
	env := runDevopsTool(t, devopsToolByName(t, ts, "port_check"), `{"port":`+port+`,"timeout_ms":2000}`)
	data := devopsPayload(t, env)
	if open, _ := data["open"].(bool); !open {
		t.Fatalf("expected the listening port to be reported open, got %v", data)
	}
}

func TestDevopsToolsAreReadOnly(t *testing.T) {
	ts := newDevopsToolset(t, t.TempDir())
	for _, tool := range ts.devopsTools() {
		ro, ok := tool.(interface{ ReadOnly() bool })
		if !ok || !ro.ReadOnly() {
			t.Fatalf("%s must be read-only: an operational inspection should never be able to make an incident worse", tool.Name())
		}
	}
}

func TestMissingRequiredArgumentIsRejected(t *testing.T) {
	ts := newDevopsToolset(t, t.TempDir())
	tool := devopsToolByName(t, ts, "k8s_logs")
	env := runDevopsTool(t, tool, "{}")
	if success, _ := env["success"].(bool); success {
		t.Fatalf("expected a missing pod name to be rejected, got %v", env)
	}
}

func TestOutputCapKeepsTheTail(t *testing.T) {
	long := strings.Repeat("a", devopsMaxOutputBytes) + "TAIL"
	out, truncated := capDevopsOutput(long)
	if !truncated {
		t.Fatal("expected oversized output to be truncated")
	}
	if !strings.HasSuffix(out, "TAIL") {
		t.Fatal("truncation must keep the end of the output: for logs the newest lines explain the failure")
	}
	if len(out) > devopsMaxOutputBytes+16 {
		t.Fatalf("truncated output is still %d bytes", len(out))
	}
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func splitHostPort(t *testing.T, addr string) (string, string) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	return host, port
}
