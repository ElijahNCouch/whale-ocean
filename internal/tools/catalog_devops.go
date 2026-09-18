package tools

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/core"
)

// Read-only operational inspections, one per line of enquiry a person would
// actually make when something is broken.
//
// A capable model does not need these: it would reach for shell_run and write
// `kubectl get pods -o wide` itself. A smaller one usually does not know the
// incantation, gets the flags wrong, or pipes the output through something
// that truncates the useful part. Naming each inspection as its own tool, with
// its arguments spelled out, turns recall into selection — which is the thing
// small models are reliably good at.
//
// Two rules keep the set honest. Every tool here only reads, so none of them
// can be the reason an incident got worse. And a tool is only offered when the
// binary behind it exists, so the catalogue a model sees is a description of
// what this machine can actually do rather than a wish list it will hallucinate
// its way through.

const (
	devopsDefaultTimeout = 30 * time.Second
	devopsMaxOutputBytes = 64 * 1024
)

// devopsCLIs are the binaries the operational tools are built on, with the
// name a person would recognise them by.
var devopsCLIs = []struct{ bin, label string }{
	{"git", "Git"},
	{"docker", "Docker"},
	{"kubectl", "Kubernetes"},
	{"helm", "Helm"},
	{"terraform", "Terraform"},
	{"gh", "GitHub CLI"},
	{"aws", "AWS CLI"},
	{"systemctl", "systemd"},
	{"journalctl", "journald"},
}

func (b *Toolset) devopsTools() []core.Tool {
	var tools []core.Tool

	// Always available: it needs no binary, and it is what tells a model which
	// of the others it is going to get.
	tools = append(tools, toolFn{
		name:        "env_info",
		description: "Report the operating system, architecture, working directory and which operational CLIs (git, docker, kubectl, helm, terraform, gh, aws, systemctl) are installed, with their versions. Call this first when you do not know what the machine can do.",
		parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
		readOnly:     true,
		capabilities: []string{"shell.read"},
		fn:           b.envInfo,
	})

	tools = append(tools, toolFn{
		name:        "port_check",
		description: "Check whether a TCP port is accepting connections. Use this to tell 'the service is down' apart from 'the service is up but answering wrongly'.",
		parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"host":       map[string]any{"type": "string", "description": "Hostname or IP. Defaults to localhost."},
				"port":       map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
				"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 30000},
			},
			"required": []string{"port"},
		},
		readOnly:     true,
		capabilities: []string{"net.read"},
		fn:           b.portCheck,
	})

	tools = append(tools, toolFn{
		name:        "dns_lookup",
		description: "Resolve a hostname to its A/AAAA addresses. Use this when a service is unreachable to tell a name-resolution failure apart from a connection failure.",
		parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"host": map[string]any{"type": "string", "description": "Hostname to resolve."},
			},
			"required": []string{"host"},
		},
		readOnly:     true,
		capabilities: []string{"net.read"},
		fn:           b.dnsLookup,
	})

	for _, spec := range devopsCommandSpecs() {
		if !binaryAvailable(spec.bin) {
			continue
		}
		tools = append(tools, b.devopsCommandTool(spec))
	}
	return tools
}

// devopsCommandSpec is one read-only invocation of a CLI, described rather
// than coded, so the set stays scannable and every entry gets the same output
// capping, timeout and workspace confinement.
type devopsCommandSpec struct {
	name        string
	bin         string
	description string
	// params are the JSON-schema properties exposed to the model.
	params map[string]any
	// required names the arguments the model must supply.
	required []string
	// argv turns validated arguments into a command line. Returning an error
	// rejects the call before anything runs.
	argv func(args map[string]any) ([]string, error)
}

func devopsCommandSpecs() []devopsCommandSpec {
	return []devopsCommandSpec{
		{
			name:        "git_status",
			bin:         "git",
			description: "Show the current branch, how far it is ahead or behind its upstream, and the staged, unstaged and untracked files.",
			params:      map[string]any{},
			argv: func(map[string]any) ([]string, error) {
				return []string{"status", "--porcelain=v2", "--branch"}, nil
			},
		},
		{
			name:        "git_log",
			bin:         "git",
			description: "Show recent commits as one line each: hash, author, relative date and subject.",
			params: map[string]any{
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "How many commits. Defaults to 20."},
				"path":  map[string]any{"type": "string", "description": "Optional path to restrict history to."},
			},
			argv: func(args map[string]any) ([]string, error) {
				out := []string{"log", "--no-color", "--date=relative",
					"--pretty=format:%h %an %ad %s", "-n", strconv.Itoa(intArg(args, "limit", 20, 1, 100))}
				if p := stringArg(args, "path"); p != "" {
					out = append(out, "--", p)
				}
				return out, nil
			},
		},
		{
			name:        "git_diff",
			bin:         "git",
			description: "Show what changed. Defaults to a per-file summary; ask for the full patch only when you need the exact lines.",
			params: map[string]any{
				"staged": map[string]any{"type": "boolean", "description": "Diff the index against HEAD instead of the working tree."},
				"full":   map[string]any{"type": "boolean", "description": "Return the whole patch instead of a per-file summary."},
				"path":   map[string]any{"type": "string", "description": "Optional path to restrict the diff to."},
			},
			argv: func(args map[string]any) ([]string, error) {
				out := []string{"diff", "--no-color"}
				if boolArg(args, "staged") {
					out = append(out, "--cached")
				}
				if !boolArg(args, "full") {
					out = append(out, "--stat")
				}
				if p := stringArg(args, "path"); p != "" {
					out = append(out, "--", p)
				}
				return out, nil
			},
		},
		{
			name:        "docker_ps",
			bin:         "docker",
			description: "List Docker containers with their image, status, health and published ports.",
			params: map[string]any{
				"all": map[string]any{"type": "boolean", "description": "Include stopped containers. Defaults to running only."},
			},
			argv: func(args map[string]any) ([]string, error) {
				out := []string{"ps", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"}
				if boolArg(args, "all") {
					out = append(out, "--all")
				}
				return out, nil
			},
		},
		{
			name:        "docker_logs",
			bin:         "docker",
			description: "Show the tail of a container's logs. Use docker_ps first to get the container name.",
			params: map[string]any{
				"container": map[string]any{"type": "string", "description": "Container name or id."},
				"lines":     map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "How many trailing lines. Defaults to 200."},
			},
			required: []string{"container"},
			argv: func(args map[string]any) ([]string, error) {
				name := stringArg(args, "container")
				if name == "" {
					return nil, fmt.Errorf("container is required")
				}
				return []string{"logs", "--tail", strconv.Itoa(intArg(args, "lines", 200, 1, 2000)), name}, nil
			},
		},
		{
			name:        "k8s_get",
			bin:         "kubectl",
			description: "List Kubernetes resources of one kind, such as pods, deployments, services, nodes or events, with their status.",
			params: map[string]any{
				"resource":  map[string]any{"type": "string", "description": "Resource kind, e.g. pods, deployments, svc, nodes, events."},
				"namespace": map[string]any{"type": "string", "description": "Namespace. Omit for the current context's namespace, or pass 'all' for every namespace."},
				"selector":  map[string]any{"type": "string", "description": "Optional label selector, e.g. app=api."},
			},
			required: []string{"resource"},
			argv: func(args map[string]any) ([]string, error) {
				res := stringArg(args, "resource")
				if res == "" {
					return nil, fmt.Errorf("resource is required")
				}
				out := []string{"get", res, "-o", "wide"}
				out = append(out, kubeNamespaceArgs(stringArg(args, "namespace"))...)
				if sel := stringArg(args, "selector"); sel != "" {
					out = append(out, "-l", sel)
				}
				return out, nil
			},
		},
		{
			name:        "k8s_describe",
			bin:         "kubectl",
			description: "Describe one Kubernetes resource in full, including recent events. This is usually what explains why a pod will not start.",
			params: map[string]any{
				"resource":  map[string]any{"type": "string", "description": "Resource kind, e.g. pod, deployment."},
				"name":      map[string]any{"type": "string", "description": "Resource name."},
				"namespace": map[string]any{"type": "string", "description": "Namespace. Omit for the current context's namespace."},
			},
			required: []string{"resource", "name"},
			argv: func(args map[string]any) ([]string, error) {
				res, name := stringArg(args, "resource"), stringArg(args, "name")
				if res == "" || name == "" {
					return nil, fmt.Errorf("resource and name are required")
				}
				out := []string{"describe", res, name}
				return append(out, kubeNamespaceArgs(stringArg(args, "namespace"))...), nil
			},
		},
		{
			name:        "k8s_logs",
			bin:         "kubectl",
			description: "Show the tail of a pod's logs, optionally from the instance that crashed rather than the one running now.",
			params: map[string]any{
				"pod":       map[string]any{"type": "string", "description": "Pod name."},
				"namespace": map[string]any{"type": "string", "description": "Namespace. Omit for the current context's namespace."},
				"container": map[string]any{"type": "string", "description": "Container name, for multi-container pods."},
				"previous":  map[string]any{"type": "boolean", "description": "Read the previous instance's logs, which is where a crash loop leaves its reason."},
				"lines":     map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "How many trailing lines. Defaults to 200."},
			},
			required: []string{"pod"},
			argv: func(args map[string]any) ([]string, error) {
				pod := stringArg(args, "pod")
				if pod == "" {
					return nil, fmt.Errorf("pod is required")
				}
				out := []string{"logs", pod, "--tail", strconv.Itoa(intArg(args, "lines", 200, 1, 2000))}
				if c := stringArg(args, "container"); c != "" {
					out = append(out, "-c", c)
				}
				if boolArg(args, "previous") {
					out = append(out, "--previous")
				}
				return append(out, kubeNamespaceArgs(stringArg(args, "namespace"))...), nil
			},
		},
		{
			name:        "k8s_context",
			bin:         "kubectl",
			description: "Show which cluster and namespace kubectl is currently pointed at. Check this before believing anything else kubectl tells you.",
			params:      map[string]any{},
			argv: func(map[string]any) ([]string, error) {
				return []string{"config", "get-contexts"}, nil
			},
		},
		{
			name:        "terraform_plan",
			bin:         "terraform",
			description: "Show what Terraform would change, without changing it. This runs a refresh-free plan and never applies.",
			params: map[string]any{
				"dir": map[string]any{"type": "string", "description": "Directory holding the Terraform configuration, relative to the workspace."},
			},
			argv: func(args map[string]any) ([]string, error) {
				out := []string{}
				if d := stringArg(args, "dir"); d != "" {
					out = append(out, "-chdir="+d)
				}
				return append(out, "plan", "-no-color", "-lock=false", "-input=false"), nil
			},
		},
		{
			name:        "gh_run_list",
			bin:         "gh",
			description: "List recent GitHub Actions runs for this repository with their status and conclusion.",
			params: map[string]any{
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "How many runs. Defaults to 10."},
				"branch": map[string]any{"type": "string", "description": "Restrict to one branch."},
			},
			argv: func(args map[string]any) ([]string, error) {
				out := []string{"run", "list", "--limit", strconv.Itoa(intArg(args, "limit", 10, 1, 50))}
				if br := stringArg(args, "branch"); br != "" {
					out = append(out, "--branch", br)
				}
				return out, nil
			},
		},
		{
			name:        "systemd_status",
			bin:         "systemctl",
			description: "Show whether a systemd unit is loaded, active and enabled, with its most recent log lines.",
			params: map[string]any{
				"unit": map[string]any{"type": "string", "description": "Unit name, e.g. nginx.service."},
			},
			required: []string{"unit"},
			argv: func(args map[string]any) ([]string, error) {
				unit := stringArg(args, "unit")
				if unit == "" {
					return nil, fmt.Errorf("unit is required")
				}
				return []string{"status", unit, "--no-pager", "--lines", "50"}, nil
			},
		},
	}
}

func kubeNamespaceArgs(ns string) []string {
	switch strings.ToLower(strings.TrimSpace(ns)) {
	case "":
		return nil
	case "all", "*", "--all-namespaces":
		return []string{"--all-namespaces"}
	default:
		return []string{"-n", ns}
	}
}

func (b *Toolset) devopsCommandTool(spec devopsCommandSpec) core.Tool {
	props := map[string]any{}
	for k, v := range spec.params {
		props[k] = v
	}
	required := append([]string(nil), spec.required...)
	sort.Strings(required)
	params := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		params["required"] = required
	}
	return toolFn{
		name:         spec.name,
		description:  spec.description,
		parameters:   params,
		readOnly:     true,
		capabilities: []string{"shell.read"},
		fn: func(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
			var args map[string]any
			if err := decodeInput(call.Input, &args); err != nil {
				return marshalToolError(call, "invalid_args", err.Error()), nil
			}
			argv, err := spec.argv(args)
			if err != nil {
				return marshalToolError(call, "invalid_args", err.Error()), nil
			}
			res := b.runDevopsCommand(ctx, spec.bin, argv)
			return marshalToolResult(call, res)
		},
	}
}

type devopsCommandResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Note     string `json:"note,omitempty"`
}

// runDevopsCommand executes the binary directly with a fixed argv. Nothing is
// passed through a shell, so an argument carrying a semicolon or a backtick is
// an argument and not a second command.
func (b *Toolset) runDevopsCommand(ctx context.Context, bin string, argv []string) devopsCommandResult {
	ctx, cancel := context.WithTimeout(ctx, devopsDefaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = b.root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()

	out, truncated := capDevopsOutput(buf.String())
	res := devopsCommandResult{
		Command:  bin + " " + strings.Join(argv, " "),
		ExitCode: cmd.ProcessState.ExitCode(),
		Output:   out,
	}
	if truncated {
		res.Note = fmt.Sprintf("output truncated to %d bytes; narrow the query or ask for fewer lines", devopsMaxOutputBytes)
	}
	if ctx.Err() != nil {
		res.Note = strings.TrimSpace(res.Note + " command timed out after " + devopsDefaultTimeout.String())
		res.ExitCode = -1
	} else if runErr != nil && res.ExitCode == 0 {
		res.ExitCode = -1
		res.Note = strings.TrimSpace(res.Note + " " + runErr.Error())
	}
	if strings.TrimSpace(res.Output) == "" && res.ExitCode == 0 {
		res.Output = "(no output)"
	}
	return res
}

func capDevopsOutput(s string) (string, bool) {
	if len(s) <= devopsMaxOutputBytes {
		return s, false
	}
	// Keep the tail: for logs and statuses the newest lines are the ones that
	// explain the failure.
	return "...\n" + s[len(s)-devopsMaxOutputBytes:], true
}

type envInfoResult struct {
	OS        string            `json:"os"`
	Arch      string            `json:"arch"`
	Workspace string            `json:"workspace"`
	Available map[string]string `json:"available_tools"`
	Missing   []string          `json:"missing_tools,omitempty"`
}

func (b *Toolset) envInfo(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	res := envInfoResult{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Workspace: b.root,
		Available: map[string]string{},
	}
	for _, c := range devopsCLIs {
		if !binaryAvailable(c.bin) {
			res.Missing = append(res.Missing, c.bin)
			continue
		}
		res.Available[c.bin] = b.binaryVersion(ctx, c.bin)
	}
	sort.Strings(res.Missing)
	return marshalToolResult(call, res)
}

// binaryVersion asks a CLI what it is, keeping only the first line: these
// commands are chatty and the version is always on the first one.
func (b *Toolset) binaryVersion(ctx context.Context, bin string) string {
	// Each CLI reports its version its own way, and two of them do not accept
	// --version at all.
	args := []string{"--version"}
	switch bin {
	case "kubectl":
		args = []string{"version", "--client"}
	case "helm":
		args = []string{"version", "--short"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return "installed"
	}
	line := firstInformativeLine(string(out))
	if line == "" {
		return "installed"
	}
	if len(line) > 120 {
		line = line[:120]
	}
	return line
}

type portCheckResult struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Open    bool   `json:"open"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (b *Toolset) portCheck(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	var args map[string]any
	if err := decodeInput(call.Input, &args); err != nil {
		return marshalToolError(call, "invalid_args", err.Error()), nil
	}
	port := intArg(args, "port", 0, 1, 65535)
	if port == 0 {
		return marshalToolError(call, "invalid_args", "port is required"), nil
	}
	host := stringArg(args, "host")
	if host == "" {
		host = "localhost"
	}
	timeout := time.Duration(intArg(args, "timeout_ms", 3000, 1, 30000)) * time.Millisecond

	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	res := portCheckResult{Host: host, Port: port}
	if err != nil {
		res.Error = err.Error()
		return marshalToolResult(call, res)
	}
	_ = conn.Close()
	res.Open = true
	res.Latency = time.Since(start).Round(time.Millisecond).String()
	return marshalToolResult(call, res)
}

type dnsLookupResult struct {
	Host      string   `json:"host"`
	Addresses []string `json:"addresses,omitempty"`
	Error     string   `json:"error,omitempty"`
}

func (b *Toolset) dnsLookup(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	var args map[string]any
	if err := decodeInput(call.Input, &args); err != nil {
		return marshalToolError(call, "invalid_args", err.Error()), nil
	}
	host := stringArg(args, "host")
	if host == "" {
		return marshalToolError(call, "invalid_args", "host is required"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	res := dnsLookupResult{Host: host, Addresses: addrs}
	if err != nil {
		res.Error = err.Error()
	}
	return marshalToolResult(call, res)
}

// firstInformativeLine skips leading lines that carry no version, such as the
// "clientVersion:" header kubectl prints before the fields.
func firstInformativeLine(out string) string {
	for _, raw := range strings.Split(strings.TrimSpace(out), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasSuffix(line, ":") {
			continue
		}
		return line
	}
	return ""
}

func binaryAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func boolArg(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func intArg(args map[string]any, key string, def, min, max int) int {
	var n int
	switch v := args[key].(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return def
		}
		n = parsed
	default:
		return def
	}
	if n < min || n > max {
		return def
	}
	return n
}
