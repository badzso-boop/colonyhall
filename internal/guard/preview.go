package guard

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ComposeServicePort holds a container port declared by a service.
type ComposeServicePort struct {
	Service       string
	ContainerPort int
	HostPort      int
}

// ComposeInfo is the parsed and validated compose file information.
type ComposeInfo struct {
	Services           []string
	Ports              []ComposeServicePort
	NeedsDefaultLimits map[string]bool
	ServicesWithPorts  map[string]bool
}

// RawCompose maps the docker-compose YAML structure for security inspection.
type RawCompose struct {
	Version  any                   `yaml:"version,omitempty"`
	Services map[string]RawService `yaml:"services"`
	Volumes  map[string]any        `yaml:"volumes,omitempty"`
	Networks map[string]any        `yaml:"networks,omitempty"`
	Include  any                   `yaml:"include,omitempty"`
	Secrets  any                   `yaml:"secrets,omitempty"`
	Configs  any                   `yaml:"configs,omitempty"`
}

// RawService maps service fields for validation.
type RawService struct {
	Image       string     `yaml:"image,omitempty"`
	Build       any        `yaml:"build,omitempty"`
	Privileged  *bool      `yaml:"privileged,omitempty"`
	CapAdd      []string   `yaml:"cap_add,omitempty"`
	CapDrop     []string   `yaml:"cap_drop,omitempty"`
	Devices     []any      `yaml:"devices,omitempty"`
	NetworkMode string     `yaml:"network_mode,omitempty"`
	Pid         string     `yaml:"pid,omitempty"`
	Ipc         string     `yaml:"ipc,omitempty"`
	UsernsMode  string     `yaml:"userns_mode,omitempty"`
	SecurityOpt []string   `yaml:"security_opt,omitempty"`
	Volumes     []any      `yaml:"volumes,omitempty"`
	Ports       []any      `yaml:"ports,omitempty"`
	Expose      []any      `yaml:"expose,omitempty"`
	Deploy      *RawDeploy `yaml:"deploy,omitempty"`
	Extends     any        `yaml:"extends,omitempty"`
	VolumesFrom []string   `yaml:"volumes_from,omitempty"`
	EnvFile     any        `yaml:"env_file,omitempty"`
}

type RawDeploy struct {
	Resources *RawResources `yaml:"resources,omitempty"`
}

type RawResources struct {
	Limits *RawLimits `yaml:"limits,omitempty"`
}

type RawLimits struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

// insideRoots reports whether dir (symlinks resolved) lies in one of roots.
func insideRoots(dir string, roots []string) bool {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	for _, r := range roots {
		if within(real, r) {
			return true
		}
	}
	return false
}

// hostRelative reports whether a path from the compose file is resolved by
// compose against something other than the compose folder: the home folder
// (~) or an environment variable ($HOME, ${PWD}); such a path cannot be
// checked here, so it is refused.
func hostRelative(p string) bool {
	return strings.HasPrefix(p, "~") || strings.Contains(p, "$")
}

// ValidateComposeFile reads, parses and validates a compose file against safety rules.
func ValidateComposeFile(composePath, rootDir string) (*ComposeInfo, error) {
	realRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		realRoot = filepath.Clean(rootDir)
	}
	realCompose, err := filepath.EvalSymlinks(composePath)
	if err != nil {
		realCompose = filepath.Clean(composePath)
	}
	if !within(realCompose, realRoot) {
		return nil, fmt.Errorf("refused: compose file %q is outside root directory", composePath)
	}

	raw, err := os.ReadFile(realCompose)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	return ValidateComposeYAML(raw, composePath, rootDir)
}

// ValidateComposeYAML parses and validates compose YAML bytes.
func ValidateComposeYAML(raw []byte, composePath, rootDir string) (*ComposeInfo, error) {
	var c RawCompose
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}
	if len(c.Services) == 0 {
		return nil, errors.New("compose file has no services")
	}
	// Other compose files, host files as secrets/configs and volume drivers
	// (driver_opts can bind any host folder) would bypass the checks below.
	if c.Include != nil {
		return nil, errors.New("refused: include is not allowed")
	}
	if c.Secrets != nil || c.Configs != nil {
		return nil, errors.New("refused: top-level secrets and configs are not allowed")
	}
	for vName, v := range c.Volumes {
		if m, ok := v.(map[string]any); ok {
			if m["driver"] != nil || m["driver_opts"] != nil || m["external"] != nil || m["name"] != nil {
				return nil, fmt.Errorf("refused: volume %q sets a driver, driver_opts, external or name", vName)
			}
		}
	}

	composeDir := filepath.Dir(composePath)
	info := &ComposeInfo{
		NeedsDefaultLimits: make(map[string]bool),
		ServicesWithPorts:  make(map[string]bool),
	}

	realRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		realRoot = filepath.Clean(rootDir)
	}

	for sName, s := range c.Services {
		info.Services = append(info.Services, sName)

		// 1. Privileged
		if s.Privileged != nil && *s.Privileged {
			return nil, fmt.Errorf("refused: service %q has privileged mode enabled", sName)
		}

		// 2. CapAdd
		if len(s.CapAdd) > 0 {
			return nil, fmt.Errorf("refused: service %q specifies cap_add", sName)
		}

		// 3. Devices
		if len(s.Devices) > 0 {
			return nil, fmt.Errorf("refused: service %q specifies devices", sName)
		}

		// 4. NetworkMode: host
		if strings.EqualFold(strings.TrimSpace(s.NetworkMode), "host") {
			return nil, fmt.Errorf("refused: service %q uses network_mode: host", sName)
		}

		// 5. Pid: host
		if strings.EqualFold(strings.TrimSpace(s.Pid), "host") {
			return nil, fmt.Errorf("refused: service %q uses pid: host", sName)
		}

		// 6. Ipc: host
		if strings.EqualFold(strings.TrimSpace(s.Ipc), "host") {
			return nil, fmt.Errorf("refused: service %q uses ipc: host", sName)
		}

		// 7. UsernsMode
		if strings.TrimSpace(s.UsernsMode) != "" {
			return nil, fmt.Errorf("refused: service %q specifies userns_mode", sName)
		}

		// 8. SecurityOpt
		if len(s.SecurityOpt) > 0 {
			return nil, fmt.Errorf("refused: service %q specifies security_opt", sName)
		}

		if s.Extends != nil || len(s.VolumesFrom) > 0 {
			return nil, fmt.Errorf("refused: service %q uses extends or volumes_from", sName)
		}
		if s.EnvFile != nil {
			var files []string
			switch ef := s.EnvFile.(type) {
			case string:
				files = append(files, ef)
			case []any:
				for _, f := range ef {
					switch fv := f.(type) {
					case string:
						files = append(files, fv)
					case map[string]any:
						p, _ := fv["path"].(string)
						files = append(files, p)
					}
				}
			}
			for _, f := range files {
				target := f
				if !filepath.IsAbs(target) {
					target = filepath.Join(composeDir, target)
				}
				if real, err := filepath.EvalSymlinks(target); err == nil {
					target = real
				}
				if f == "" || hostRelative(f) || !within(filepath.Clean(target), realRoot) {
					return nil, fmt.Errorf("refused: service %q env_file %q is outside root directory", sName, f)
				}
			}
		}

		// 9. Build context
		if s.Build != nil {
			var bContext string
			switch b := s.Build.(type) {
			case string:
				bContext = b
			case map[string]any:
				if ctxVal, ok := b["context"].(string); ok {
					bContext = ctxVal
				}
			}
			if hostRelative(bContext) || strings.Contains(bContext, "://") {
				return nil, fmt.Errorf("refused: service %q build context %q is not a folder inside the root", sName, bContext)
			}
			if bContext != "" {
				target := bContext
				if !filepath.IsAbs(target) {
					target = filepath.Join(composeDir, target)
				}
				realTarget, err := filepath.EvalSymlinks(target)
				if err != nil {
					realTarget = filepath.Clean(target)
				}
				if !within(realTarget, realRoot) {
					return nil, fmt.Errorf("refused: service %q build context %q is outside root directory", sName, bContext)
				}
			}
		}

		// 10. Volumes and bind mounts
		for _, v := range s.Volumes {
			var src string
			var isBind bool
			switch val := v.(type) {
			case string:
				parts := strings.Split(val, ":")
				if len(parts) > 0 {
					sPart := strings.TrimSpace(parts[0])
					if strings.Contains(sPart, "docker.sock") {
						return nil, fmt.Errorf("refused: service %q mounts docker socket", sName)
					}
					// If it looks like a path (starts with /, ./, ../, ~ or contains separator)
					if strings.HasPrefix(sPart, "/") || strings.HasPrefix(sPart, ".") || strings.HasPrefix(sPart, "~") || strings.Contains(sPart, "/") || strings.Contains(sPart, `\`) {
						src = sPart
						isBind = true
					}
				}
			case map[string]any:
				if sVal, ok := val["source"].(string); ok {
					if strings.Contains(sVal, "docker.sock") {
						return nil, fmt.Errorf("refused: service %q mounts docker socket", sName)
					}
					vType, _ := val["type"].(string)
					if vType == "bind" || strings.HasPrefix(sVal, "/") || strings.HasPrefix(sVal, ".") {
						src = sVal
						isBind = true
					}
				}
			}

			if isBind && hostRelative(src) {
				return nil, fmt.Errorf("refused: service %q volume %q uses ~ or a variable", sName, src)
			}
			if isBind && src != "" {
				target := src
				if !filepath.IsAbs(target) {
					target = filepath.Join(composeDir, target)
				}
				realTarget, err := filepath.EvalSymlinks(target)
				if err != nil {
					realTarget = filepath.Clean(target)
				}
				if !within(realTarget, realRoot) {
					return nil, fmt.Errorf("refused: service %q volume %q is outside root directory", sName, src)
				}
			}
		}

		// 11. Container ports
		if len(s.Ports) > 0 {
			info.ServicesWithPorts[sName] = true
		}
		for _, p := range s.Ports {
			cPort := parseContainerPort(p)
			if cPort > 0 {
				info.Ports = append(info.Ports, ComposeServicePort{
					Service:       sName,
					ContainerPort: cPort,
				})
			}
		}
		for _, e := range s.Expose {
			cPort := parseContainerPort(e)
			if cPort > 0 {
				already := false
				for _, existing := range info.Ports {
					if existing.Service == sName && existing.ContainerPort == cPort {
						already = true
						break
					}
				}
				if !already {
					info.Ports = append(info.Ports, ComposeServicePort{
						Service:       sName,
						ContainerPort: cPort,
					})
				}
			}
		}

		// 12. Resource limits
		hasLimits := s.Deploy != nil && s.Deploy.Resources != nil && s.Deploy.Resources.Limits != nil &&
			s.Deploy.Resources.Limits.CPUs != "" && s.Deploy.Resources.Limits.Memory != ""
		if !hasLimits {
			info.NeedsDefaultLimits[sName] = true
		}
	}

	return info, nil
}

func parseContainerPort(p any) int {
	switch val := p.(type) {
	case int:
		return val
	case string:
		// e.g. "80", "8080:80", "127.0.0.1:8080:80", "80/tcp"
		str := strings.TrimSpace(val)
		if idx := strings.Index(str, "/"); idx != -1 {
			str = str[:idx]
		}
		parts := strings.Split(str, ":")
		last := parts[len(parts)-1]
		if n, err := strconv.Atoi(last); err == nil && n > 0 && n <= 65535 {
			return n
		}
	case map[string]any:
		if t, ok := val["target"].(int); ok {
			return t
		}
	}
	return 0
}

// PortPool manages per-agent port allocation from a configured range.
type PortPool struct {
	startPort int
	endPort   int
	numSlots  int
	mu        sync.Mutex
	used      map[int]string // port -> previewID
	// Free, if set, reports whether a port can still be bound on this
	// machine: another program (e.g. a second guard of another Linux user
	// with an overlapping preview_ports range) may hold it.
	Bindable func(port int) bool
}

// NewPortPool initializes a port pool from a range string (e.g. "41000-41999").
func NewPortPool(rangeStr string) (*PortPool, error) {
	if rangeStr == "" {
		rangeStr = "41000-41999"
	}
	parts := strings.Split(strings.TrimSpace(rangeStr), "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port range %q", rangeStr)
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid port range start %q", parts[0])
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid port range end %q", parts[1])
	}
	if start <= 0 || end <= start || end > 65535 {
		return nil, fmt.Errorf("invalid port range %d-%d", start, end)
	}
	total := end - start + 1
	numSlots := total / 10
	if numSlots <= 0 {
		numSlots = 1
	}
	return &PortPool{
		startPort: start,
		endPort:   end,
		numSlots:  numSlots,
		used:      make(map[int]string),
	}, nil
}

// Allocate assigns available ports within the agent's 10-port block for a preview.
func (p *PortPool) Allocate(agentID, previewID string, count int) ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	h := fnv.New32a()
	h.Write([]byte(agentID))
	slot := int(h.Sum32()) % p.numSlots
	basePort := p.startPort + slot*10

	var allocated []int
	taken := 0
	for port := basePort; port < basePort+10 && len(allocated) < count; port++ {
		if _, inUse := p.used[port]; inUse {
			continue
		}
		if p.Bindable != nil && !p.Bindable(port) {
			taken++
			continue
		}
		allocated = append(allocated, port)
	}
	if len(allocated) < count {
		if taken > 0 {
			return nil, fmt.Errorf("not enough free preview ports for agent (have %d, needed %d in range %d-%d; %d held by another program, e.g. another guard on this machine: give each guard its own preview_ports range)", len(allocated), count, basePort, basePort+9, taken)
		}
		return nil, fmt.Errorf("not enough free preview ports for agent (have %d, needed %d in range %d-%d)", len(allocated), count, basePort, basePort+9)
	}
	for _, port := range allocated {
		p.used[port] = previewID
	}
	return allocated, nil
}

// MarkUsed marks a port as allocated to a preview (e.g. after supervisor restart).
func (p *PortPool) MarkUsed(port int, previewID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used[port] = previewID
}

// Free releases all ports used by a preview.
func (p *PortPool) Free(previewID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port, id := range p.used {
		if id == previewID {
			delete(p.used, port)
		}
	}
}

// IsUsed reports whether a port is currently allocated.
func (p *PortPool) IsUsed(port int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.used[port]
	return ok
}

// GenerateOverrideContent generates docker-compose override YAML bytes.
func GenerateOverrideContent(previewHost string, ports []ComposeServicePort, defaultLimits map[string]bool, servicesWithPorts map[string]bool) ([]byte, error) {
	servicesMapNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	// Group ports by service
	portsByService := make(map[string][]ComposeServicePort)
	for _, sp := range ports {
		portsByService[sp.Service] = append(portsByService[sp.Service], sp)
	}

	// Determine all services needing override entries (either ports or limits)
	allServices := make(map[string]bool)
	for s := range portsByService {
		allServices[s] = true
	}
	for s := range defaultLimits {
		allServices[s] = true
	}
	for s := range servicesWithPorts {
		allServices[s] = true
	}

	var sortedServices []string
	for s := range allServices {
		sortedServices = append(sortedServices, s)
	}
	slices.Sort(sortedServices)

	for _, sName := range sortedServices {
		sNode := &yaml.Node{
			Kind: yaml.MappingNode,
		}

		// Ports: replace with !override tag
		spList := portsByService[sName]
		if len(spList) > 0 || servicesWithPorts[sName] {
			sNode.Content = append(sNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "ports"},
			)
			portsSeq := &yaml.Node{
				Kind: yaml.SequenceNode,
				Tag:  "!override",
			}
			for _, sp := range spList {
				portsSeq.Content = append(portsSeq.Content, &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: fmt.Sprintf("%s:%d:%d", previewHost, sp.HostPort, sp.ContainerPort),
				})
			}
			sNode.Content = append(sNode.Content, portsSeq)
		}

		// Default limits
		if defaultLimits[sName] {
			sNode.Content = append(sNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "deploy"},
				&yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "resources"},
						{
							Kind: yaml.MappingNode,
							Content: []*yaml.Node{
								{Kind: yaml.ScalarNode, Value: "limits"},
								{
									Kind: yaml.MappingNode,
									Content: []*yaml.Node{
										{Kind: yaml.ScalarNode, Value: "cpus"},
										{Kind: yaml.ScalarNode, Value: "1.0"},
										{Kind: yaml.ScalarNode, Value: "memory"},
										{Kind: yaml.ScalarNode, Value: "512M"},
									},
								},
							},
						},
					},
				},
			)
		}

		if len(sNode.Content) > 0 {
			servicesMapNode.Content = append(servicesMapNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: sName},
				sNode,
			)
		}
	}

	docNode := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "services"},
					servicesMapNode,
				},
			},
		},
	}

	return yaml.Marshal(docNode)
}

// DockerRunner runs docker compose commands.
type DockerRunner interface {
	Up(ctx context.Context, project string, composePath, overridePath string) error
	Down(ctx context.Context, project string, composePath, overridePath string) error
}

// ExecDockerRunner runs real docker compose commands via exec.CommandContext.
type ExecDockerRunner struct {
	Timeout time.Duration
}

func (r *ExecDockerRunner) Up(ctx context.Context, project, composePath, overridePath string) error {
	to := r.Timeout
	if to <= 0 {
		to = 2 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	cmd := exec.CommandContext(cctx, "docker", "compose", "-p", project, "-f", composePath, "-f", overridePath, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up (%w): %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *ExecDockerRunner) Down(ctx context.Context, project, composePath, overridePath string) error {
	to := r.Timeout
	if to <= 0 {
		to = time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	cmd := exec.CommandContext(cctx, "docker", "compose", "-p", project, "-f", composePath, "-f", overridePath, "down")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down (%w): %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// FakeDockerRunner is a mock implementation for tests.
type FakeDockerRunner struct {
	mu        sync.Mutex
	UpCalls   []string
	DownCalls []string
	UpErr     error
	DownErr   error
}

func (f *FakeDockerRunner) Up(_ context.Context, project, composePath, overridePath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.UpCalls = append(f.UpCalls, fmt.Sprintf("%s|%s|%s", project, composePath, overridePath))
	return f.UpErr
}

func (f *FakeDockerRunner) Down(_ context.Context, project, composePath, overridePath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DownCalls = append(f.DownCalls, fmt.Sprintf("%s|%s|%s", project, composePath, overridePath))
	return f.DownErr
}

// activePreview tracks a running preview on this host.
type activePreview struct {
	ID           string
	AgentID      string
	Project      string
	ComposePath  string
	OverridePath string
	URL          string
	Ports        []int
	ExpiresAt    time.Time
}

// PreviewSupervisor reconciles previews on the host.
type PreviewSupervisor struct {
	Client      *Client
	Runner      DockerRunner
	Roots       []string
	PreviewHost string
	Ports       *PortPool
	// Dir holds the compose override files, one folder per preview. The
	// guard sets it to its own state folder; empty means a per-user folder
	// in the system temp dir.
	Dir    string
	mu     sync.Mutex
	active map[string]*activePreview
	Log    *slog.Logger
}

// NewPreviewSupervisor initializes the preview supervisor.
func NewPreviewSupervisor(client *Client, runner DockerRunner, roots []string, previewHost, previewPorts string, logger *slog.Logger) (*PreviewSupervisor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if previewHost == "" {
		previewHost = "127.0.0.1"
	}
	pool, err := NewPortPool(previewPorts)
	if err != nil {
		return nil, err
	}
	return &PreviewSupervisor{
		Client:      client,
		Runner:      runner,
		Roots:       roots,
		PreviewHost: previewHost,
		Ports:       pool,
		active:      make(map[string]*activePreview),
		Log:         logger,
	}, nil
}

func (s *PreviewSupervisor) dir() string {
	if s.Dir != "" {
		return s.Dir
	}
	// Not the shared /tmp/colony-previews: another user's guard could own it.
	return filepath.Join(os.TempDir(), fmt.Sprintf("colony-previews-%d", os.Getuid()))
}

// recoveredOverride is the override file of a preview started before a
// guard restart. The shared /tmp/colony-previews of older guards is no
// longer read (round 40): any local user can create files there, and the
// guard would hand them to docker compose.
func (s *PreviewSupervisor) recoveredOverride(id string) string {
	return filepath.Join(s.dir(), id, "docker-compose.override.yaml")
}

// Reconcile handles requested and running previews from the host config.
func (s *PreviewSupervisor) Reconcile(ctx context.Context, previews []HostPreview, agents []HostAgent, haltedAgents map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	agentMap := make(map[string]HostAgent, len(agents))
	for _, a := range agents {
		agentMap[a.AgentID] = a
	}

	seenServerIDs := make(map[string]bool)

	for _, p := range previews {
		seenServerIDs[p.ID] = true
		a, hasAgent := agentMap[p.AgentID]
		halted := haltedAgents[p.AgentID]

		switch p.State {
		case "requested":
			if !hasAgent {
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", "agent is not assigned to this host")
				continue
			}
			if halted {
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", "agent is currently halted")
				continue
			}
			if !a.AllowPreview {
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", "previews are not allowed for this agent")
				continue
			}
			if a.Sandbox || a.Mode != "direct" {
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", "previews need a full-access agent")
				continue
			}
			if a.RootDir == "" {
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", "agent has no root directory configured")
				continue
			}
			// The server cannot widen the machine's roots: the agent's root
			// comes from the server, so check it against guard.yaml here.
			if !insideRoots(a.RootDir, s.Roots) {
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", "agent root is outside the root folders of this machine")
				continue
			}

			// Validate compose path inside agent root
			composePath := filepath.Join(a.RootDir, p.ComposePath)
			info, err := ValidateComposeFile(composePath, a.RootDir)
			if err != nil {
				s.Log.Warn("guard: preview compose validation failed", "preview", p.Name, "err", err)
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", err.Error())
				continue
			}

			// Allocate ports
			portCount := len(info.Ports)
			if portCount == 0 {
				// Allocate at least 1 port for primary service if no ports declared
				portCount = 1
				info.Ports = append(info.Ports, ComposeServicePort{
					Service:       info.Services[0],
					ContainerPort: 80,
				})
			}
			allocated, err := s.Ports.Allocate(p.AgentID, p.ID, portCount)
			if err != nil {
				s.Log.Warn("guard: port allocation failed", "preview", p.Name, "err", err)
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", err.Error())
				continue
			}
			for i := range info.Ports {
				info.Ports[i].HostPort = allocated[i]
			}

			// Generate override file
			overrideContent, err := GenerateOverrideContent(s.PreviewHost, info.Ports, info.NeedsDefaultLimits, info.ServicesWithPorts)
			if err != nil {
				s.Ports.Free(p.ID)
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", fmt.Sprintf("generate override: %s", err))
				continue
			}

			overrideDir := filepath.Join(s.dir(), p.ID)
			if err := os.MkdirAll(overrideDir, 0700); err != nil {
				s.Ports.Free(p.ID)
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", fmt.Sprintf("create override dir: %s", err))
				continue
			}
			overridePath := filepath.Join(overrideDir, "docker-compose.override.yaml")
			if err := os.WriteFile(overridePath, overrideContent, 0600); err != nil {
				s.Ports.Free(p.ID)
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", fmt.Sprintf("write override: %s", err))
				continue
			}

			project := fmt.Sprintf("colony-%s-%s", a.Handle, p.Name)
			if err := s.Runner.Up(ctx, project, composePath, overridePath); err != nil {
				s.Ports.Free(p.ID)
				_ = os.RemoveAll(overrideDir)
				s.Log.Warn("guard: docker compose up failed", "preview", p.Name, "err", err)
				_ = s.Client.ReportPreview(ctx, p.ID, "failed", "", err.Error())
				continue
			}

			url := fmt.Sprintf("http://%s:%d", s.PreviewHost, allocated[0])
			s.active[p.ID] = &activePreview{
				ID:           p.ID,
				AgentID:      p.AgentID,
				Project:      project,
				ComposePath:  composePath,
				OverridePath: overridePath,
				URL:          url,
				Ports:        allocated,
				ExpiresAt:    p.ExpiresAt,
			}
			_ = s.Client.ReportPreview(ctx, p.ID, "running", url, "")

		case "running":
			act, isAct := s.active[p.ID]
			now := time.Now()
			// Stop if expired or agent halted
			if now.After(p.ExpiresAt) || halted {
				s.stopPreview(ctx, p.ID, act)
				_ = s.Client.ReportPreview(ctx, p.ID, "stopped", "", "")
				continue
			}
			// If not active locally (e.g. guard restart), recover it
			if !isAct && hasAgent {
				composePath := filepath.Join(a.RootDir, p.ComposePath)
				overridePath := s.recoveredOverride(p.ID)
				project := fmt.Sprintf("colony-%s-%s", a.Handle, p.Name)
				actPrev := &activePreview{
					ID:           p.ID,
					AgentID:      p.AgentID,
					Project:      project,
					ComposePath:  composePath,
					OverridePath: overridePath,
					URL:          p.URL,
					ExpiresAt:    p.ExpiresAt,
				}
				if p.URL != "" {
					if u, err := url.Parse(p.URL); err == nil && u.Port() != "" {
						if portNum, err := strconv.Atoi(u.Port()); err == nil {
							s.Ports.MarkUsed(portNum, p.ID)
							actPrev.Ports = []int{portNum}
						}
					}
				}
				s.active[p.ID] = actPrev
			}

		case "stopped", "failed":
			if act, isAct := s.active[p.ID]; isAct {
				s.stopPreview(ctx, p.ID, act)
			}
		}
	}

	// Stop any locally active previews that are no longer reported by the server
	for id, act := range s.active {
		if !seenServerIDs[id] {
			s.stopPreview(ctx, id, act)
		}
	}
}

// CheckExpired checks active previews and stops any whose expiry has passed.
func (s *PreviewSupervisor) CheckExpired(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, act := range s.active {
		if now.After(act.ExpiresAt) {
			s.stopPreview(ctx, id, act)
			_ = s.Client.ReportPreview(ctx, id, "stopped", "", "")
		}
	}
}

// CheckHalted checks active previews and stops any belonging to halted agents.
func (s *PreviewSupervisor) CheckHalted(ctx context.Context, haltedAgents map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, act := range s.active {
		if haltedAgents[act.AgentID] {
			s.stopPreview(ctx, id, act)
			_ = s.Client.ReportPreview(ctx, id, "stopped", "", "")
		}
	}
}

func (s *PreviewSupervisor) stopPreview(ctx context.Context, id string, act *activePreview) {
	if act != nil {
		_ = s.Runner.Down(ctx, act.Project, act.ComposePath, act.OverridePath)
		if act.OverridePath != "" {
			_ = os.RemoveAll(filepath.Dir(act.OverridePath))
		}
	}
	s.Ports.Free(id)
	delete(s.active, id)
}

// StopAll stops all active previews.
func (s *PreviewSupervisor) StopAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, act := range s.active {
		s.stopPreview(ctx, id, act)
	}
}
